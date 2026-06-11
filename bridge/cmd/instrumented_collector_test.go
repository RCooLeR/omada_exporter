package cmd

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

type constCollector struct {
	desc  *prometheus.Desc
	value float64
}

func newConstCollector(name string, value float64) constCollector {
	return constCollector{
		desc:  prometheus.NewDesc(name, "test metric", nil, nil),
		value: value,
	}
}

func (c constCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c constCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, c.value)
}

type panicCollector struct{}

func (panicCollector) Describe(chan<- *prometheus.Desc) {}

func (panicCollector) Collect(chan<- prometheus.Metric) {
	panic("boom")
}

func TestInstrumentedCollectorRecoversPanic(t *testing.T) {
	health := newCollectorHealth()
	instrumented := newInstrumentedCollector("panic", panicCollector{}, health)
	instrumented.Collect(make(chan prometheus.Metric))

	registry := prometheus.NewRegistry()
	registry.MustRegister(health)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() returned error: %v", err)
	}

	values := map[string]float64{}
	for _, family := range families {
		for _, metric := range family.Metric {
			switch {
			case metric.Gauge != nil:
				values[family.GetName()] = metric.Gauge.GetValue()
			case metric.Counter != nil:
				values[family.GetName()] = metric.Counter.GetValue()
			}
		}
	}

	if values["omada_collector_last_scrape_completed"] != 0 {
		t.Fatalf("last scrape completed = %v, want 0", values["omada_collector_last_scrape_completed"])
	}
	if values["omada_collector_panics_total"] != 1 {
		t.Fatalf("panics total = %v, want 1", values["omada_collector_panics_total"])
	}
	if values["omada_collector_last_scrape_duration_seconds"] < 0 {
		t.Fatalf("duration = %v, want non-negative", values["omada_collector_last_scrape_duration_seconds"])
	}
}

func TestInstrumentedCollectorsCanShareRegistry(t *testing.T) {
	health := newCollectorHealth()
	registry := prometheus.NewRegistry()

	registry.MustRegister(
		health,
		newInstrumentedCollector("first", newConstCollector("test_first_metric", 1), health),
		newInstrumentedCollector("second", newConstCollector("test_second_metric", 2), health),
	)

	if _, err := registry.Gather(); err != nil {
		t.Fatalf("Gather() returned error: %v", err)
	}
}
