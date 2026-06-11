package cmd

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

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

// instrumentedCollector wraps a real collector with exporter self-metrics.
//
// Prometheus collectors do not return errors from Collect; they can only emit
// metrics or panic. The wrapper therefore stays deliberately small: it measures
// how long the collector took and catches panics so one broken collector does
// not take down the whole scrape.
type instrumentedCollector struct {
	name       string
	delegate   prometheus.Collector
	panicCount atomic.Uint64
}

func newInstrumentedCollector(name string, delegate prometheus.Collector) prometheus.Collector {
	return &instrumentedCollector{name: name, delegate: delegate}
}

func (c *instrumentedCollector) Describe(ch chan<- *prometheus.Desc) {
	c.delegate.Describe(ch)
	ch <- collectorCompletedDesc
	ch <- collectorDurationDesc
	ch <- collectorPanicsDesc
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

		ch <- prometheus.MustNewConstMetric(collectorCompletedDesc, prometheus.GaugeValue, completed, c.name)
		ch <- prometheus.MustNewConstMetric(collectorDurationDesc, prometheus.GaugeValue, time.Since(start).Seconds(), c.name)
		ch <- prometheus.MustNewConstMetric(collectorPanicsDesc, prometheus.CounterValue, float64(c.panicCount.Load()), c.name)
	}()

	c.delegate.Collect(ch)
}
