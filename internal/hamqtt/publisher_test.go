package hamqtt

import (
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func TestObjectIDIgnoresDynamicDeviceLabels(t *testing.T) {
	first := objectID("omada_device_cpu_percentage", map[string]string{
		"site_id":                 "site-1",
		"device_mac":              "aa:bb:cc:dd:ee:ff",
		"device_name":             "Switch",
		"device_status":           "Connected",
		"device_ip":               "192.168.1.10",
		"device_version":          "1.0.0",
		"device_version_upgrade":  "1.0.0",
		"device_firmware_version": "1.0.0",
	})

	second := objectID("omada_device_cpu_percentage", map[string]string{
		"site_id":                 "site-1",
		"device_mac":              "aa:bb:cc:dd:ee:ff",
		"device_name":             "Renamed Switch",
		"device_status":           "Disconnected",
		"device_ip":               "192.168.1.11",
		"device_version":          "1.0.1",
		"device_version_upgrade":  "1.0.1",
		"device_firmware_version": "1.0.1",
	})

	if first != second {
		t.Fatalf("object id changed for dynamic labels: %q != %q", first, second)
	}
}

func TestObjectIDIncludesPortQualifier(t *testing.T) {
	port1 := objectID("omada_port_link_status", map[string]string{
		"site_id":    "site-1",
		"device_mac": "aa:bb:cc:dd:ee:ff",
		"port":       "1",
	})
	port2 := objectID("omada_port_link_status", map[string]string{
		"site_id":    "site-1",
		"device_mac": "aa:bb:cc:dd:ee:ff",
		"port":       "2",
	})

	if port1 == port2 {
		t.Fatalf("object ids should differ by port: %q", port1)
	}
}

func TestObjectIDForVpnStatsIgnoresVPNID(t *testing.T) {
	withoutID := objectID("omada_vpn_down_bytes", map[string]string{
		"site_id":        "site-1",
		"name":           "Slobidska",
		"interface_name": "WAN/LAN4",
		"local_ip":       "10.8.0.10",
		"remote_ip":      "10.8.0.9",
	})

	withID := objectID("omada_vpn_down_bytes", map[string]string{
		"site_id":        "site-1",
		"vpn_id":         "6928d343fab6126a9a7d4ed8",
		"name":           "Slobidska",
		"interface_name": "WAN/LAN4",
		"local_ip":       "10.8.0.10",
		"remote_ip":      "10.8.0.9",
	})

	if withoutID != withID {
		t.Fatalf("vpn stats object id changed after enriching vpn_id: %q != %q", withoutID, withID)
	}
}

func TestDeviceLabelsAddsVPNIDToStatsMetrics(t *testing.T) {
	ctx := buildPublishContext([]*dto.MetricFamily{
		{
			Name: stringPtr("omada_vpn_status"),
			Metric: []*dto.Metric{
				{
					Label: []*dto.LabelPair{
						labelPair("vpn_id", "6928d343fab6126a9a7d4ed8"),
						labelPair("name", "Slobidska"),
						labelPair("vpn_mode", "Client"),
						labelPair("vpn_type", "OpenVPN"),
					},
				},
			},
		},
	})

	labels := deviceLabels("omada_vpn_down_bytes", map[string]string{
		"name":           "Slobidska",
		"interface_name": "WAN/LAN4",
		"vpn_mode":       "Client",
		"vpn_type":       "OpenVPN",
		"local_ip":       "10.8.0.10",
		"remote_ip":      "10.8.0.9",
	}, ctx)

	if labels["vpn_id"] != "6928d343fab6126a9a7d4ed8" {
		t.Fatalf("expected vpn_id to be enriched, got %q", labels["vpn_id"])
	}
}

func TestRecordRateSampleBitsPerSecond(t *testing.T) {
	publisher := &Publisher{
		metricSamples: map[string]metricSample{},
	}

	start := time.Unix(100, 0).UTC()
	if rate := publisher.recordRateSample("vpn-1", 1000, start); rate != 0 {
		t.Fatalf("first sample should not produce rate, got %v", rate)
	}

	rate := publisher.recordRateSample("vpn-1", 1500, start.Add(2*time.Second))
	if rate != 2000 {
		t.Fatalf("expected 2000 bit/s, got %v", rate)
	}
}

func stringPtr(value string) *string {
	return &value
}

func labelPair(name, value string) *dto.LabelPair {
	return &dto.LabelPair{
		Name:  stringPtr(name),
		Value: stringPtr(value),
	}
}
