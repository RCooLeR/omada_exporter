package app

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestBridgeMetricIsolationAndLegacyLabels(t *testing.T) {
	for _, id := range []string{"", "slobidska44", "another-bridge"} {
		t.Run("id="+id, func(t *testing.T) {
			registry, err := newExporterRegistry(false, true, id)
			if err != nil {
				t.Fatal(err)
			}
			registerer := bridgeRegisterer(registry, id)
			gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "omada_test_value", Help: "Test"})
			gauge.Set(7)
			if err := registerer.Register(gauge); err != nil {
				t.Fatal(err)
			}
			health := newCollectorHealth()
			if err := registerer.Register(health); err != nil {
				t.Fatal(err)
			}
			handler := promhttp.InstrumentMetricHandler(registerer, newMetricsHandler(registry, registerer))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
			if response.Code != 200 {
				t.Fatalf("scrape: %d", response.Code)
			}
			families, err := registry.Gather()
			if err != nil {
				t.Fatal(err)
			}
			for _, family := range families {
				for _, metric := range family.Metric {
					found := false
					for _, label := range metric.Label {
						if label.GetName() == "bridge_id" {
							found = true
							if label.GetValue() != id {
								t.Fatalf("%s belongs to %q, want %q", family.GetName(), label.GetValue(), id)
							}
						}
					}
					if found != (id != "") {
						t.Fatalf("%s namespace label presence = %v for bridge %q", family.GetName(), found, id)
					}
				}
			}
			// Per-collector endpoints use the same registerer boundary.
			partial := prometheus.NewRegistry()
			if err := bridgeRegisterer(partial, id).Register(gauge); err != nil {
				t.Fatal(err)
			}
			out := httptest.NewRecorder()
			newMetricsHandler(partial, bridgeRegisterer(partial, id)).ServeHTTP(out, httptest.NewRequest("GET", "/metrics/device", nil))
			if out.Code != 200 || !strings.Contains(out.Body.String(), "omada_test_value") {
				t.Fatal("partial metrics missing")
			}
			if id != "" && !strings.Contains(out.Body.String(), `bridge_id="`+id+`"`) {
				t.Fatal("partial endpoint missing bridge ID")
			}
		})
	}
}
