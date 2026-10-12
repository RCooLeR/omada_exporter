package hamqtt

import (
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestQualityEntityIdentityAndQualifiers(t *testing.T) {
	for _, tc := range []struct{ name, key, a, b string }{
		{"omada_ap_uplink_dropped_packets_total", "direction", "up", "down"},
		{"omada_ap_interference_percentage", "radio_band", "2.4", "5"},
		{"omada_network_quality_available", "source", "ap_uplink", "wan"},
	} {
		a := map[string]string{"site_id": "site", "device_mac": "ap", tc.key: tc.a, "device_name": "before"}
		b := copyLabels(a)
		b[tc.key] = tc.b
		if objectID(tc.name, a) == objectID(tc.name, b) {
			t.Errorf("%s identities collide", tc.name)
		}
		b = copyLabels(a)
		b["device_name"] = "after"
		if objectID(tc.name, a) != objectID(tc.name, b) {
			t.Errorf("%s rename changed identity", tc.name)
		}
		if !strings.Contains(friendlyMetricName(tc.name, a), tc.a) {
			t.Errorf("%s missing display qualifier", tc.name)
		}
	}
}

func TestQualitySensorUnits(t *testing.T) {
	for _, tc := range []struct{ name, unit string }{
		{"omada_gateway_wan_quality_latency_average_seconds", "s"},
		{"omada_gateway_wan_quality_jitter_average_seconds", "s"},
		{"omada_gateway_wan_quality_packet_loss_average_percentage", "%"},
		{"omada_ap_interference_percentage", "%"},
	} {
		if got := sensorHints(tc.name, dto.MetricType_GAUGE)["unit_of_measurement"]; got != tc.unit {
			t.Errorf("%s unit=%v", tc.name, got)
		}
	}
	if got := sensorHints("omada_ap_uplink_dropped_packets_total", dto.MetricType_COUNTER)["state_class"]; got != "total_increasing" {
		t.Errorf("packet state_class=%v", got)
	}
}
