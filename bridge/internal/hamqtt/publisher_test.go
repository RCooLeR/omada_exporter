package hamqtt

import (
	"reflect"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestParseTrackedClientMACs(t *testing.T) {
	got := parseTrackedClientMACs("AA:BB:CC:DD:EE:FF, aa-bb-cc-dd-ee-ff;112233445566, aabb.ccdd.eeff, invalid")
	want := []string{"aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseTrackedClientMACs() = %#v, want %#v", got, want)
	}
}

func TestConfiguredClientTrackers(t *testing.T) {
	publisher := &Publisher{
		client: &api.Client{
			Config: &config.Config{
				MQTTTopicPrefix: "omada_exporter",
				Site:            "Default",
			},
			SiteId: "site-id",
		},
		trackedClientMACs: []string{"aa:bb:cc:dd:ee:ff"},
	}

	trackers := publisher.configuredClientTrackers()
	tracker, ok := trackers["aa_bb_cc_dd_ee_ff"]
	if !ok {
		t.Fatalf("configuredClientTrackers() missing tracker for configured MAC")
	}

	if tracker.StateTopic != "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/state" {
		t.Fatalf("StateTopic = %q", tracker.StateTopic)
	}
	if tracker.AttributesTopic != "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/attributes" {
		t.Fatalf("AttributesTopic = %q", tracker.AttributesTopic)
	}

	wantLabels := map[string]string{
		"mac":     "aa:bb:cc:dd:ee:ff",
		"site":    "Default",
		"site_id": "site-id",
	}
	if !reflect.DeepEqual(tracker.Labels, wantLabels) {
		t.Fatalf("Labels = %#v, want %#v", tracker.Labels, wantLabels)
	}
}
