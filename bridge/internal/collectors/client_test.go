package collector

import (
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

func TestClientMetricLabelsSeparateWiredAttachmentFromClient(t *testing.T) {
	labels := clientMetricLabels(model.NetworkClient{
		Mac:        "aa:bb:cc:dd:ee:ff",
		Wireless:   false,
		SwitchMac:  "11:22:33:44:55:66",
		SwitchName: "Core Switch",
		Port:       1,
		LagId:      0,
		WifiMode:   0,
		ApMac:      "stale-ap",
		ApName:     "stale-ap",
		Ssid:       "stale-ssid",
	}, "Default", "site-id")

	for index, want := range map[int]string{
		13: "11:22:33:44:55:66",
		14: "Core Switch",
		15: "1",
		16: "",
		17: "false",
		18: "",
		19: "",
		20: "",
		21: "",
	} {
		if labels[index] != want {
			t.Fatalf("labels[%d] = %q, want %q", index, labels[index], want)
		}
	}
}

func TestClientMetricLabelsSeparateWirelessAttachmentFromSwitch(t *testing.T) {
	labels := clientMetricLabels(model.NetworkClient{
		Mac:        "aa:bb:cc:dd:ee:ff",
		Wireless:   true,
		SwitchMac:  "stale-switch",
		SwitchName: "stale-switch",
		Port:       8,
		LagId:      3,
		WifiMode:   9,
		ApMac:      "11:22:33:44:55:66",
		ApName:     "Office AP",
		Ssid:       "Office",
	}, "Default", "site-id")

	for index, want := range map[int]string{
		13: "",
		14: "",
		15: "",
		16: "",
		17: "true",
		18: "11:22:33:44:55:66",
		19: "Office AP",
		20: "802.11bea",
		21: "Office",
	} {
		if labels[index] != want {
			t.Fatalf("labels[%d] = %q, want %q", index, labels[index], want)
		}
	}
}
