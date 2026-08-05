package hamqtt

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	dto "github.com/prometheus/client_model/go"
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

func TestPublishConfiguredClientTrackerOffline(t *testing.T) {
	client := &api.Client{
		Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
			MQTTRetain:          true,
			Site:                "Default",
		},
		SiteId: "site-id",
	}
	publisher := &Publisher{
		client:            client,
		mqtt:              &recordingMQTTClient{messages: map[string][]byte{}},
		availabilityTopic: "omada_exporter/status",
		published:         map[string]struct{}{},
		trackedClientMACs: []string{"aa:bb:cc:dd:ee:ff"},
	}

	publisher.publishClientTrackers(map[string]clientTracker{})
	mqttClient := publisher.mqtt.(*recordingMQTTClient)

	stateTopic := "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/state"
	if got := string(mqttClient.messages[stateTopic]); got != "not_home" {
		t.Fatalf("state topic payload = %q, want not_home", got)
	}

	discoveryTopic := "homeassistant/device_tracker/omada_exporter/aa_bb_cc_dd_ee_ff/config"
	var discovery map[string]any
	if err := json.Unmarshal(mqttClient.messages[discoveryTopic], &discovery); err != nil {
		t.Fatalf("failed to decode discovery payload: %v", err)
	}

	for key, want := range map[string]any{
		"name":         "aa:bb:cc:dd:ee:ff",
		"unique_id":    "omada_client_aa_bb_cc_dd_ee_ff",
		"object_id":    "omada_client_aa_bb_cc_dd_ee_ff",
		"state_topic":  stateTopic,
		"source_type":  "router",
		"payload_home": "home",
	} {
		if discovery[key] != want {
			t.Fatalf("discovery[%s] = %#v, want %#v", key, discovery[key], want)
		}
	}

	attributesTopic := "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/attributes"
	var attributes map[string]any
	if err := json.Unmarshal(mqttClient.messages[attributesTopic], &attributes); err != nil {
		t.Fatalf("failed to decode attributes payload: %v", err)
	}
	if _, ok := attributes["last_updated"]; ok {
		t.Fatal("configured tracker attributes contain last_updated")
	}
	if _, ok := attributes["last_seen"]; ok {
		t.Fatal("never-seen configured tracker attributes contain last_seen")
	}
	if attributes["configured"] != true {
		t.Fatalf("configured attribute = %#v, want true", attributes["configured"])
	}

	publisher.publishClientTrackers(map[string]clientTracker{})
	if got := mqttClient.publishCounts[attributesTopic]; got != 1 {
		t.Fatalf("unchanged configured attributes published %d times, want 1", got)
	}
}

func TestClientTrackerAttributesPublishOnlyOnChange(t *testing.T) {
	mqttClient := &recordingMQTTClient{messages: map[string][]byte{}}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
			MQTTRetain:          true,
		}},
		mqtt:               mqttClient,
		published:          map[string]struct{}{},
		knownClients:       map[string]clientTracker{},
		lastClientTrackers: map[string]clientTracker{},
	}
	id := "aa_bb_cc_dd_ee_ff"
	tracker := clientTracker{
		StateTopic:      "omada_exporter/device_trackers/" + id + "/state",
		AttributesTopic: "omada_exporter/device_trackers/" + id + "/attributes",
		Labels: map[string]string{
			"mac":       "aa:bb:cc:dd:ee:ff",
			"host_name": "phone",
			"vendor":    "Example",
		},
	}

	publisher.publishClientTrackers(map[string]clientTracker{id: tracker})
	publisher.publishClientTrackers(map[string]clientTracker{id: tracker})
	if got := mqttClient.publishCounts[tracker.AttributesTopic]; got != 1 {
		t.Fatalf("unchanged online attributes published %d times, want 1", got)
	}
	if got := mqttClient.publishCounts[tracker.StateTopic]; got != 2 {
		t.Fatalf("online presence state published %d times, want 2", got)
	}
	if got := string(mqttClient.messages[tracker.StateTopic]); got != "home" {
		t.Fatalf("online presence state = %q, want home", got)
	}

	var attributes map[string]any
	if err := json.Unmarshal(mqttClient.messages[tracker.AttributesTopic], &attributes); err != nil {
		t.Fatalf("failed to decode attributes payload: %v", err)
	}
	for _, key := range []string{"last_seen", "last_updated"} {
		if _, ok := attributes[key]; ok {
			t.Fatalf("online tracker attributes contain %s", key)
		}
	}

	changed := tracker
	changed.Labels = copyLabels(tracker.Labels)
	changed.Labels["host_name"] = "renamed-phone"
	publisher.publishClientTrackers(map[string]clientTracker{id: changed})
	if got := mqttClient.publishCounts[tracker.AttributesTopic]; got != 2 {
		t.Fatalf("changed online attributes published %d times, want 2", got)
	}
}

func TestClientTrackerAttributesReplayWhenRetainIsDisabled(t *testing.T) {
	mqttClient := &recordingMQTTClient{messages: map[string][]byte{}}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
			MQTTRetain:          false,
		}},
		mqtt:               mqttClient,
		published:          map[string]struct{}{},
		knownClients:       map[string]clientTracker{},
		lastClientTrackers: map[string]clientTracker{},
	}
	id := "aa_bb_cc_dd_ee_ff"
	tracker := clientTracker{
		StateTopic:      "omada_exporter/device_trackers/" + id + "/state",
		AttributesTopic: "omada_exporter/device_trackers/" + id + "/attributes",
		Labels:          map[string]string{"mac": "aa:bb:cc:dd:ee:ff", "host_name": "phone"},
	}

	publisher.publishClientTrackers(map[string]clientTracker{id: tracker})
	publisher.publishClientTrackers(map[string]clientTracker{id: tracker})

	if got := mqttClient.publishCounts[tracker.AttributesTopic]; got != 2 {
		t.Fatalf("non-retained attributes published %d times, want 2", got)
	}
}

func TestClientTrackerLastSeenPublishesOnceWhenClientLeaves(t *testing.T) {
	mqttClient := &recordingMQTTClient{messages: map[string][]byte{}}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
			MQTTRetain:          true,
		}},
		mqtt:               mqttClient,
		published:          map[string]struct{}{},
		knownClients:       map[string]clientTracker{},
		lastClientTrackers: map[string]clientTracker{},
	}
	id := "aa_bb_cc_dd_ee_ff"
	tracker := clientTracker{
		StateTopic:      "omada_exporter/device_trackers/" + id + "/state",
		AttributesTopic: "omada_exporter/device_trackers/" + id + "/attributes",
		Labels:          map[string]string{"mac": "aa:bb:cc:dd:ee:ff", "host_name": "phone"},
	}

	publisher.publishClientTrackers(map[string]clientTracker{id: tracker})
	wantLastSeen := publisher.clientLastSeen[id].Format(time.RFC3339)
	publisher.publishClientTrackers(map[string]clientTracker{})

	if got := string(mqttClient.messages[tracker.StateTopic]); got != "not_home" {
		t.Fatalf("tracker state = %q, want not_home", got)
	}
	var attributes map[string]any
	if err := json.Unmarshal(mqttClient.messages[tracker.AttributesTopic], &attributes); err != nil {
		t.Fatalf("failed to decode attributes payload: %v", err)
	}
	if got := attributes["last_seen"]; got != wantLastSeen {
		t.Fatalf("last_seen = %#v, want %q", got, wantLastSeen)
	}
	if got := mqttClient.publishCounts[tracker.AttributesTopic]; got != 2 {
		t.Fatalf("transition attributes published %d times, want 2", got)
	}

	publisher.publishClientTrackers(map[string]clientTracker{})
	if got := mqttClient.publishCounts[tracker.AttributesTopic]; got != 2 {
		t.Fatalf("offline attributes republished %d times, want 2", got)
	}
}

func TestObjectIDKeepsClientIdentityStableAcrossAttachmentChanges(t *testing.T) {
	oldLabels := map[string]string{
		"site":        "Default",
		"site_id":     "site-id",
		"mac":         "aa:bb:cc:dd:ee:ff",
		"gateway_mac": "10:20:30:40:50:60",
		"port":        "8",
		"lag_id":      "3",
		"wifi_mode":   "802.11a",
		"ssid":        "stale",
	}
	currentLabels := map[string]string{
		"site":        "Default",
		"site_id":     "site-id",
		"mac":         "aa:bb:cc:dd:ee:ff",
		"gateway_mac": "60:50:40:30:20:10",
		"port":        "1",
		"lag_id":      "",
	}

	oldID := objectID("omada_client_traffic_down_bytes", oldLabels)
	currentID := objectID("omada_client_traffic_down_bytes", currentLabels)
	if oldID != currentID {
		t.Fatalf("client object IDs differ across attachment changes: %q != %q", oldID, currentID)
	}
}

func TestObjectIDDistinguishesRealSubresourcesAndDPIProperties(t *testing.T) {
	baseDevice := map[string]string{
		"site_id":    "site-id",
		"device_mac": "aa:bb:cc:dd:ee:ff",
		"port":       "1",
	}
	otherPort := copyLabels(baseDevice)
	otherPort["port"] = "2"
	if objectID("omada_port_link_status", baseDevice) == objectID("omada_port_link_status", otherPort) {
		t.Fatal("different physical ports received the same object ID")
	}

	categoryOne := map[string]string{"site_id": "site-id", "family_id": "1", "family_name": "Business"}
	categoryTwo := map[string]string{"site_id": "site-id", "family_id": "2", "family_name": "Streaming"}
	if objectID("omada_dpi_category_traffic_bytes", categoryOne) == objectID("omada_dpi_category_traffic_bytes", categoryTwo) {
		t.Fatal("different DPI categories received the same object ID")
	}

	applicationOne := map[string]string{"site_id": "site-id", "family_id": "1", "application_id": "10"}
	applicationTwo := map[string]string{"site_id": "site-id", "family_id": "1", "application_id": "11"}
	if objectID("omada_dpi_application_traffic_bytes", applicationOne) == objectID("omada_dpi_application_traffic_bytes", applicationTwo) {
		t.Fatal("different DPI applications received the same object ID")
	}
}

func TestObjectIDIgnoresSiteDisplayNameWhenSiteIDExists(t *testing.T) {
	before := map[string]string{"site_id": "site-id", "site": "Old name", "device_mac": "aa:bb:cc:dd:ee:ff"}
	after := map[string]string{"site_id": "site-id", "site": "New name", "device_mac": "aa:bb:cc:dd:ee:ff"}
	if objectID("omada_device_cpu_percentage", before) != objectID("omada_device_cpu_percentage", after) {
		t.Fatal("site display-name change altered object ID despite stable site ID")
	}
}

func TestReconcileSupersededEntitiesOnlyDeletesConfirmedCounterpart(t *testing.T) {
	mqttClient := &recordingMQTTClient{messages: map[string][]byte{}}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTRetain: true,
		}},
		mqtt:              mqttClient,
		published:         map[string]struct{}{},
		retainedDiscovery: map[string]string{},
		retainedStates:    map[string]map[string]string{},
		retainedLoaded:    true,
	}

	oldDiscovery := "homeassistant/sensor/omada_exporter/old/config"
	oldState := "omada_exporter/entities/old/state"
	publisher.retainedDiscovery[oldDiscovery] = oldState
	publisher.retainedStates[oldState] = map[string]string{
		"metric":      "omada_client_traffic_down_bytes",
		"site":        "Default",
		"site_id":     "old-site-id",
		"mac":         "aa:bb:cc:dd:ee:ff",
		"gateway_mac": "10:20:30:40:50:60",
		"port":        "8",
		"lag_id":      "3",
	}

	currentEntity := entity{
		DiscoveryTopic: "homeassistant/sensor/omada_exporter/current/config",
		StateTopic:     "omada_exporter/entities/current/state",
		MetricName:     "omada_client_traffic_down_bytes",
		Labels: map[string]string{
			"site":    "Default",
			"site_id": "new-site-id",
			"mac":     "aa:bb:cc:dd:ee:ff",
			"port":    "1",
			"lag_id":  "",
		},
	}
	publisher.reconcileSupersededEntities(map[string]entity{currentEntity.DiscoveryTopic: currentEntity})

	if payload, ok := mqttClient.messages[oldDiscovery]; !ok || len(payload) != 0 {
		t.Fatalf("old discovery topic was not tombstoned: present=%v payload=%q", ok, payload)
	}
	if payload, ok := mqttClient.messages[oldState]; !ok || len(payload) != 0 {
		t.Fatalf("old state topic was not tombstoned: present=%v payload=%q", ok, payload)
	}
}

func TestReconcileSupersededEntitiesRetriesFailedTombstone(t *testing.T) {
	oldDiscovery := "homeassistant/sensor/omada_exporter/old/config"
	oldState := "omada_exporter/entities/old/state"
	mqttClient := &recordingMQTTClient{
		messages:        map[string][]byte{},
		publishFailures: map[string]int{oldDiscovery: 1},
	}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{MQTTRetain: true}},
		mqtt:   mqttClient,
		published: map[string]struct{}{
			oldDiscovery: {},
		},
		retainedDiscovery: map[string]string{oldDiscovery: oldState},
		retainedStates: map[string]map[string]string{
			oldState: {
				"metric":  "omada_client_traffic_down_bytes",
				"site_id": "site-id",
				"mac":     "aa:bb:cc:dd:ee:ff",
			},
		},
		retainedLoaded: true,
	}
	currentEntity := entity{
		DiscoveryTopic: "homeassistant/sensor/omada_exporter/current/config",
		StateTopic:     "omada_exporter/entities/current/state",
		MetricName:     "omada_client_traffic_down_bytes",
		Labels:         map[string]string{"site_id": "site-id", "mac": "aa:bb:cc:dd:ee:ff"},
	}
	current := map[string]entity{currentEntity.DiscoveryTopic: currentEntity}

	publisher.reconcileSupersededEntities(current)
	if _, ok := publisher.retainedDiscovery[oldDiscovery]; !ok {
		t.Fatal("failed tombstone was removed from retained discovery inventory")
	}
	if _, ok := publisher.retainedStates[oldState]; !ok {
		t.Fatal("failed tombstone state was removed from retained state inventory")
	}
	if _, ok := publisher.published[oldDiscovery]; !ok {
		t.Fatal("failed tombstone discovery was removed from published inventory")
	}

	publisher.reconcileSupersededEntities(current)
	if _, ok := publisher.retainedDiscovery[oldDiscovery]; ok {
		t.Fatal("successful retry left discovery in retained inventory")
	}
	if _, ok := publisher.retainedStates[oldState]; ok {
		t.Fatal("successful retry left state in retained inventory")
	}
	if _, ok := publisher.published[oldDiscovery]; ok {
		t.Fatal("successful retry left discovery in published inventory")
	}
	if got := mqttClient.publishCounts[oldDiscovery]; got != 2 {
		t.Fatalf("discovery tombstone attempts = %d, want 2", got)
	}
}

func TestCanonicalEntityTopicsReuseNewestRetainedCounterpart(t *testing.T) {
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
		}},
		retainedDiscovery: map[string]string{
			"homeassistant/sensor/omada_exporter/stale/config":   "omada_exporter/entities/stale/state",
			"homeassistant/sensor/omada_exporter/current/config": "omada_exporter/entities/current/state",
		},
		retainedStates: map[string]map[string]string{
			"omada_exporter/entities/stale/state": {
				"metric":       "omada_client_traffic_down_bytes",
				"last_updated": "2026-07-14T10:00:00Z",
				"site":         "Default",
				"site_id":      "old-site-id",
				"mac":          "aa:bb:cc:dd:ee:ff",
				"port":         "8",
				"lag_id":       "3",
			},
			"omada_exporter/entities/current/state": {
				"metric":       "omada_client_traffic_down_bytes",
				"last_updated": "2026-07-19T10:00:00Z",
				"site":         "Default",
				"site_id":      "new-site-id",
				"mac":          "aa:bb:cc:dd:ee:ff",
				"port":         "1",
				"lag_id":       "0",
			},
		},
		retainedLoaded: true,
	}

	objectID, discoveryTopic, stateTopic := publisher.canonicalEntityTopics(
		"omada_client_traffic_down_bytes",
		map[string]string{
			"site":    "Default",
			"site_id": "new-site-id",
			"mac":     "aa:bb:cc:dd:ee:ff",
			"port":    "1",
		},
		"sensor",
		"new",
		"homeassistant/sensor/omada_exporter/new/config",
		"omada_exporter/entities/new/state",
	)

	if objectID != "current" || discoveryTopic != "homeassistant/sensor/omada_exporter/current/config" || stateTopic != "omada_exporter/entities/current/state" {
		t.Fatalf("canonical topics = (%q, %q, %q), want newest retained counterpart", objectID, discoveryTopic, stateTopic)
	}
}

func TestReconcileSupersededEntitiesPreservesUnconfirmedDifferentSite(t *testing.T) {
	mqttClient := &recordingMQTTClient{messages: map[string][]byte{}}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTRetain: true,
		}},
		mqtt:              mqttClient,
		published:         map[string]struct{}{},
		retainedDiscovery: map[string]string{"old/config": "old/state"},
		retainedStates: map[string]map[string]string{
			"old/state": {
				"metric":  "omada_client_traffic_down_bytes",
				"site":    "Other site",
				"site_id": "other-site-id",
				"mac":     "aa:bb:cc:dd:ee:ff",
			},
		},
		retainedLoaded: true,
	}
	currentEntity := entity{
		DiscoveryTopic: "current/config",
		StateTopic:     "current/state",
		MetricName:     "omada_client_traffic_down_bytes",
		Labels:         map[string]string{"site": "Default", "site_id": "site-id", "mac": "aa:bb:cc:dd:ee:ff"},
	}
	publisher.reconcileSupersededEntities(map[string]entity{currentEntity.DiscoveryTopic: currentEntity})
	if _, deleted := mqttClient.messages["old/config"]; deleted {
		t.Fatal("unconfirmed entity from a different site was deleted")
	}
}

func TestInfrastructureClientMetricsUseInfrastructureDevice(t *testing.T) {
	metricName := "omada_controller_uptime_seconds"
	labelName := "device_mac"
	labelValue := "aa:bb:cc:dd:ee:ff"
	ctx := buildPublishContext([]*dto.MetricFamily{{
		Name: &metricName,
		Metric: []*dto.Metric{{Label: []*dto.LabelPair{{
			Name:  &labelName,
			Value: &labelValue,
		}}}},
	}})

	labels := map[string]string{"mac": labelValue, "name": "OC220", "device_type": "controller"}
	enriched := deviceLabels("omada_client_traffic_down_bytes", labels, ctx)
	if enriched["device_mac"] != labelValue {
		t.Fatalf("device_mac = %q, want infrastructure MAC", enriched["device_mac"])
	}

	device := deviceInfo(&api.Client{Config: &config.Config{}}, "omada_client_traffic_down_bytes", enriched)
	identifiers, ok := device["identifiers"].([]string)
	if !ok || len(identifiers) != 1 || identifiers[0] != "omada_device_aa_bb_cc_dd_ee_ff" {
		t.Fatalf("device identifiers = %#v, want infrastructure device identifier", device["identifiers"])
	}
}

func TestRemoveInfrastructureClientTrackerTombstonesDuplicate(t *testing.T) {
	mqttClient := &recordingMQTTClient{messages: map[string][]byte{}}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
		}},
		mqtt:               mqttClient,
		knownClients:       map[string]clientTracker{},
		lastClientTrackers: map[string]clientTracker{},
	}
	publisher.removeInfrastructureClientTrackers(publishContext{
		infrastructureByMAC: map[string]map[string]string{"aa_bb_cc_dd_ee_ff": {}},
	})

	for _, topic := range []string{
		"homeassistant/device_tracker/omada_exporter/aa_bb_cc_dd_ee_ff/config",
		"omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/state",
		"omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/attributes",
	} {
		payload, ok := mqttClient.messages[topic]
		if !ok || len(payload) != 0 {
			t.Fatalf("topic %q was not tombstoned: present=%v payload=%q", topic, ok, payload)
		}
	}
}

func TestRemoveInfrastructureClientTrackerPreservesExplicitConfiguration(t *testing.T) {
	mqttClient := &recordingMQTTClient{messages: map[string][]byte{}}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
		}},
		mqtt:               mqttClient,
		trackedClientMACs:  []string{"aa:bb:cc:dd:ee:ff"},
		knownClients:       map[string]clientTracker{},
		lastClientTrackers: map[string]clientTracker{},
	}
	publisher.removeInfrastructureClientTrackers(publishContext{
		infrastructureByMAC: map[string]map[string]string{"aa_bb_cc_dd_ee_ff": {}},
	})
	if len(mqttClient.messages) != 0 {
		t.Fatalf("explicitly configured infrastructure tracker was tombstoned: %#v", mqttClient.messages)
	}
}

func TestRetainedClientTrackerRestoresDynamicPresence(t *testing.T) {
	id, tracker, ok := retainedClientTracker(
		"omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/state",
		"omada_exporter",
	)
	if !ok {
		t.Fatal("retained tracker topic was not recognized")
	}
	if id != "aa_bb_cc_dd_ee_ff" {
		t.Fatalf("tracker ID = %q, want aa_bb_cc_dd_ee_ff", id)
	}
	if tracker.StateTopic != "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/state" {
		t.Fatalf("state topic = %q", tracker.StateTopic)
	}
	if tracker.AttributesTopic != "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/attributes" {
		t.Fatalf("attributes topic = %q", tracker.AttributesTopic)
	}

	if _, _, ok := retainedClientTracker("omada_exporter/device_trackers/nested/client/state", "omada_exporter"); ok {
		t.Fatal("nested tracker topic was accepted")
	}
	if id, ok := retainedClientTrackerAttributesID(
		"omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/attributes",
		"omada_exporter",
	); !ok || id != "aa_bb_cc_dd_ee_ff" {
		t.Fatalf("retained attributes topic id = %q, accepted=%v", id, ok)
	}
	if _, ok := retainedClientTrackerAttributesID(
		"omada_exporter/device_trackers/nested/client/attributes",
		"omada_exporter",
	); ok {
		t.Fatal("nested tracker attributes topic was accepted")
	}
}

func TestLoadRetainedInventoryMarksAbsentClientNotHome(t *testing.T) {
	const trackerStateTopic = "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/state"
	const trackerAttributesTopic = "omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/attributes"
	mqttClient := &recordingMQTTClient{
		messages: map[string][]byte{},
		retainedMessages: map[string][]mqtt.Message{
			"omada_exporter/device_trackers/+/state": {
				retainedMessage{topic: trackerStateTopic, payload: []byte("home")},
			},
			"omada_exporter/device_trackers/+/attributes": {
				retainedMessage{
					topic:   trackerAttributesTopic,
					payload: []byte(`{"host_name":"phone","last_seen":"2026-07-31T10:00:00Z","mac":"aa:bb:cc:dd:ee:ff"}`),
				},
			},
		},
	}
	publisher := &Publisher{
		client: &api.Client{Config: &config.Config{
			MQTTDiscoveryPrefix: "homeassistant",
			MQTTTopicPrefix:     "omada_exporter",
			MQTTRetain:          true,
		}},
		mqtt:               mqttClient,
		published:          map[string]struct{}{},
		knownClients:       map[string]clientTracker{},
		lastClientTrackers: map[string]clientTracker{},
		retainedDiscovery:  map[string]string{},
		retainedStates:     map[string]map[string]string{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := publisher.loadRetainedInventory(ctx); err != nil {
		t.Fatalf("loadRetainedInventory() error = %v", err)
	}
	if _, ok := publisher.knownClients["aa_bb_cc_dd_ee_ff"]; !ok {
		t.Fatal("retained home tracker was not restored")
	}
	if got := publisher.knownClients["aa_bb_cc_dd_ee_ff"].Labels["host_name"]; got != "phone" {
		t.Fatalf("retained tracker host_name = %q, want phone", got)
	}

	publisher.publishClientTrackers(map[string]clientTracker{})
	if got := string(mqttClient.messages[trackerStateTopic]); got != "not_home" {
		t.Fatalf("tracker state = %q, want not_home", got)
	}
	if got := mqttClient.publishCounts[trackerAttributesTopic]; got != 0 {
		t.Fatalf("unchanged retained attributes published %d times, want 0", got)
	}
}

type recordingMQTTClient struct {
	messages         map[string][]byte
	publishCounts    map[string]int
	publishFailures  map[string]int
	retainedMessages map[string][]mqtt.Message
}

func (c *recordingMQTTClient) IsConnected() bool { return true }

func (c *recordingMQTTClient) IsConnectionOpen() bool { return true }

func (c *recordingMQTTClient) Connect() mqtt.Token { return completedToken{} }

func (c *recordingMQTTClient) Disconnect(uint) {}

func (c *recordingMQTTClient) Publish(topic string, _ byte, _ bool, payload any) mqtt.Token {
	if c.messages == nil {
		c.messages = map[string][]byte{}
	}
	if c.publishCounts == nil {
		c.publishCounts = map[string]int{}
	}
	c.publishCounts[topic]++
	if c.publishFailures[topic] > 0 {
		c.publishFailures[topic]--
		return completedToken{err: errors.New("simulated mqtt publish failure")}
	}
	switch typed := payload.(type) {
	case []byte:
		c.messages[topic] = append([]byte{}, typed...)
	case string:
		c.messages[topic] = []byte(typed)
	default:
		c.messages[topic] = []byte{}
	}
	return completedToken{}
}

func (c *recordingMQTTClient) Subscribe(filter string, _ byte, handler mqtt.MessageHandler) mqtt.Token {
	for _, message := range c.retainedMessages[filter] {
		handler(c, message)
	}
	return completedToken{}
}

func (c *recordingMQTTClient) SubscribeMultiple(map[string]byte, mqtt.MessageHandler) mqtt.Token {
	return completedToken{}
}

func (c *recordingMQTTClient) Unsubscribe(...string) mqtt.Token { return completedToken{} }

func (c *recordingMQTTClient) AddRoute(string, mqtt.MessageHandler) {}

func (c *recordingMQTTClient) OptionsReader() mqtt.ClientOptionsReader {
	return mqtt.ClientOptionsReader{}
}

type completedToken struct {
	err error
}

func (completedToken) Wait() bool { return true }

func (completedToken) WaitTimeout(time.Duration) bool { return true }

func (completedToken) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func (t completedToken) Error() error { return t.err }

type retainedMessage struct {
	topic   string
	payload []byte
}

func (retainedMessage) Duplicate() bool { return false }
func (retainedMessage) Qos() byte       { return 0 }
func (retainedMessage) Retained() bool  { return true }
func (m retainedMessage) Topic() string { return m.topic }
func (retainedMessage) MessageID() uint16 {
	return 0
}
func (m retainedMessage) Payload() []byte { return m.payload }
func (retainedMessage) Ack()              {}
