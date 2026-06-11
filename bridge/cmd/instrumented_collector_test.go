package cmd

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

type panicCollector struct{}

func (panicCollector) Describe(chan<- *prometheus.Desc) {}

func (panicCollector) Collect(chan<- prometheus.Metric) {
	panic("boom")
}

func TestInstrumentedCollectorRecoversPanic(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(newInstrumentedCollector("panic", panicCollector{}))

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
