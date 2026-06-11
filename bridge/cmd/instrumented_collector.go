package cmd

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// collectorState is the last known health snapshot for one named collector.
type collectorState struct {
	completed  float64
	duration   float64
	panicCount uint64
}

// collectorHealth publishes exporter self-metrics for every wrapped collector.
//
// The health metrics live in this single collector instead of each wrapped
// collector. Prometheus registries reject duplicate descriptors when several
// collectors all describe the same metric name, so a shared publisher keeps the
// registry valid while still exposing one time series per collector label.
type collectorHealth struct {
	mu     sync.RWMutex
	states map[string]collectorState
}

var (
	collectorCompletedDesc = prometheus.NewDesc(
		"omada_collector_last_scrape_completed",
		"Whether the named Omada collector finished its last scrape without panicking. Collector API errors that are handled internally by the collector are logged by that collector.",
		[]string{"collector"},
		nil,
	)
	collectorDurationDesc = prometheus.NewDesc(
		"omada_collector_last_scrape_duration_seconds",
		"How long the named Omada collector took to finish its last scrape.",
		[]string{"collector"},
		nil,
	)
	collectorPanicsDesc = prometheus.NewDesc(
		"omada_collector_panics_total",
		"Total number of panics recovered from the named Omada collector.",
		[]string{"collector"},
		nil,
	)
)

func newCollectorHealth() *collectorHealth {
	return &collectorHealth{
		states: map[string]collectorState{},
	}
}

func (h *collectorHealth) track(name string) {
	h.mu.Lock()
	if _, ok := h.states[name]; !ok {
		h.states[name] = collectorState{}
	}
	h.mu.Unlock()
}

func (h *collectorHealth) record(name string, completed float64, duration time.Duration, panicCount uint64) {
	h.mu.Lock()
	h.states[name] = collectorState{
		completed:  completed,
		duration:   duration.Seconds(),
		panicCount: panicCount,
	}
	h.mu.Unlock()
}

func (h *collectorHealth) Describe(ch chan<- *prometheus.Desc) {
	ch <- collectorCompletedDesc
	ch <- collectorDurationDesc
	ch <- collectorPanicsDesc
}

func (h *collectorHealth) Collect(ch chan<- prometheus.Metric) {
	h.mu.RLock()
	states := make(map[string]collectorState, len(h.states))
	for name, state := range h.states {
		states[name] = state
	}
	h.mu.RUnlock()

	for name, state := range states {
		ch <- prometheus.MustNewConstMetric(collectorCompletedDesc, prometheus.GaugeValue, state.completed, name)
		ch <- prometheus.MustNewConstMetric(collectorDurationDesc, prometheus.GaugeValue, state.duration, name)
		ch <- prometheus.MustNewConstMetric(collectorPanicsDesc, prometheus.CounterValue, float64(state.panicCount), name)
	}
}

// instrumentedCollector wraps a real collector with exporter self-metrics.
//
// Prometheus collectors do not return errors from Collect; they can only emit
// metrics or panic. The wrapper therefore stays deliberately small: it measures
// how long the collector took and catches panics so one broken collector does
// not take down the whole scrape.
type instrumentedCollector struct {
	name       string
	delegate   prometheus.Collector
	health     *collectorHealth
	panicCount atomic.Uint64
}

func newInstrumentedCollector(name string, delegate prometheus.Collector, health *collectorHealth) prometheus.Collector {
	health.track(name)
	return &instrumentedCollector{name: name, delegate: delegate, health: health}
}

func (c *instrumentedCollector) Describe(ch chan<- *prometheus.Desc) {
	c.delegate.Describe(ch)
}

func (c *instrumentedCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	completed := 1.0

	defer func() {
		if recovered := recover(); recovered != nil {
			completed = 0
			c.panicCount.Add(1)
			log.Error().
				Str("collector", c.name).
				Str("panic", fmt.Sprint(recovered)).
				Msg("recovered panic from collector")
		}

		c.health.record(c.name, completed, time.Since(start), c.panicCount.Load())
	}()

	c.delegate.Collect(ch)
}
