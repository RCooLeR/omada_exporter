package cmd

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestBridgeIDValidation(t *testing.T) {
	for _, id := range []string{"", "a", "slobidska44", "apartment-2", strings.Repeat("a", 64)} {
		if err := config.ValidateBridgeID(id); err != nil {
			t.Errorf("valid bridge ID %q rejected: %v", id, err)
		}
	}
	for _, id := range []string{" ", "Apartment", "apartment_2", "-a", "a-", "a/b", "a+b", "a#b", strings.Repeat("a", 65)} {
		if err := config.ValidateBridgeID(id); err == nil {
			t.Errorf("invalid bridge ID %q accepted", id)
		}
	}
}

func TestBridgeIDCLIAndLegacyDefaults(t *testing.T) {
	for _, id := range []string{"", "slobidska44"} {
		t.Run("id="+id, func(t *testing.T) {
			t.Setenv("OMADA_BRIDGE_ID", id)
			t.Setenv("OMADA_MQTT_CLIENT_ID", "omada_exporter")
			t.Setenv("OMADA_MQTT_TOPIC_PREFIX", "omada_exporter")
			conf := &config.Config{}
			command := newCLICommand(conf)
			command.Writer, command.ErrWriter = io.Discard, io.Discard
			if err := command.Run(context.Background(), []string{"omada_exporter", "version"}); err != nil {
				t.Fatal(err)
			}
			if conf.BridgeID != id || conf.MQTTClientID != "omada_exporter" || conf.MQTTTopicPrefix != "omada_exporter" {
				t.Fatal("bridge ID parsing must not mutate base MQTT configuration")
			}
		})
	}
	command := newCLICommand(&config.Config{})
	command.Writer, command.ErrWriter = io.Discard, io.Discard
	if err := command.Run(context.Background(), []string{"omada_exporter", "--bridge-id", "a/b", "version"}); err == nil {
		t.Fatal("invalid namespace passed CLI validation")
	}
}

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
