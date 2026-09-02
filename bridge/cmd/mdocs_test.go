package cmd

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestParseDescSupportsPrometheusUnitField(t *testing.T) {
	desc := prometheus.NewDesc(
		"omada_test_temperature_celsius",
		"Test metric.",
		[]string{"site", "device_mac"},
		nil,
	)

	name, help, labels := parseDesc(desc.String())
	if name != "omada_test_temperature_celsius" {
		t.Fatalf("name = %q", name)
	}
	if help != "Test metric." {
		t.Fatalf("help = %q", help)
	}
	if labels != "site,device_mac" {
		t.Fatalf("labels = %q", labels)
	}
}
