package hamqtt

import "testing"

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
