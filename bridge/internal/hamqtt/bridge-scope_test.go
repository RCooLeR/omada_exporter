package hamqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func scopeTestPublisher(t *testing.T, bridgeID string, broker mqtt.Client) *Publisher {
	t.Helper()
	client := &session.Client{Config: &config.Config{
		BridgeID: bridgeID, MQTTBroker: "mqtt:1883", MQTTClientID: "omada_exporter",
		MQTTTopicPrefix: "omada_exporter", MQTTDiscoveryPrefix: "homeassistant", MQTTRetain: true,
		Site: "Default", MQTTTrackedClientMACs: "aa:bb:cc:dd:ee:ff",
	}}
	client.SetContextIDs("same-controller-id", "same-site-id")
	pub, err := NewPublisher(client, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub.mqtt = broker
	return pub
}

func TestBridgeScopeLegacyAndApartmentShareBrokerWithoutChangingEachOther(t *testing.T) {
	broker := &recordingMQTTClient{}
	legacy := scopeTestPublisher(t, "", broker)
	apartment := scopeTestPublisher(t, "slobidska44", broker)
	labels := map[string]string{"mac": "aa:bb:cc:dd:ee:ff", "site_id": "same-site-id", "site": "Default", "name": "Same phone"}
	tracker, ok := legacy.clientTracker("omada_client_signal_pct", labels)
	if !ok {
		t.Fatal("missing legacy tracker")
	}
	legacy.publishClientTrackers(map[string]clientTracker{legacy.clientTrackerID(labels): tracker})
	legacy.mqttClientOptions().OnConnect(broker)
	legacyMessages := map[string][]byte{}
	for topic, body := range broker.messages {
		legacyMessages[topic] = append([]byte{}, body...)
	}

	apartment.publishClientTrackers(nil) // Same phone is offline in the apartment.
	apartment.mqttClientOptions().OnConnect(broker)
	apartment.publishBytes(apartment.availabilityTopic, []byte("offline"), true)
	for topic, body := range legacyMessages {
		if !reflect.DeepEqual(broker.messages[topic], body) || broker.publishCounts[topic] != 1 {
			t.Fatalf("apartment changed legacy topic %q", topic)
		}
	}
	if string(broker.messages[tracker.StateTopic]) != "home" {
		t.Fatal("legacy phone was marked away")
	}
	for _, apptTracker := range apartment.configuredClientTrackers() {
		if string(broker.messages[apptTracker.StateTopic]) != "not_home" {
			t.Fatal("apartment phone should be away")
		}
		var attrs map[string]any
		if err := json.Unmarshal(broker.messages[apptTracker.AttributesTopic], &attrs); err != nil {
			t.Fatal(err)
		}
		if attrs["bridge_id"] != "slobidska44" || attrs["site_id"] != "same-site-id" {
			t.Fatalf("configured tracker scope missing: %#v", attrs)
		}
	}
	if legacy.availabilityTopic != "omada_exporter/status" || apartment.availabilityTopic != "omada_exporter_slobidska44/status" {
		t.Fatal("availability topics are not backward-compatible sibling namespaces")
	}
	if string(broker.messages[legacy.availabilityTopic]) != "online" {
		t.Fatal("apartment disconnect changed legacy availability")
	}
	if legacy.mqttClientOptions().ClientID == apartment.mqttClientOptions().ClientID {
		t.Fatal("publishers would disconnect each other's MQTT sessions")
	}
}

func TestBridgeScopeMetricDerivedDeviceAndTrackerIdentities(t *testing.T) {
	broker := &recordingMQTTClient{}
	legacy := scopeTestPublisher(t, "", broker)
	apartment := scopeTestPublisher(t, "slobidska44", broker)
	other := scopeTestPublisher(t, "other-flat", broker)
	name := "omada_device_cpu_percentage"
	family := &dto.MetricFamily{Name: &name}
	labels := map[string]string{"device_mac": "aa:bb:cc:dd:ee:ff", "site_id": "same-site-id", "site": "Default"}
	entities := []entity{legacy.newMetricEntity(family, labels, publishContext{}), apartment.newMetricEntity(family, labels, publishContext{}), other.newMetricEntity(family, labels, publishContext{})}
	for index, ent := range entities {
		for prev := range index {
			old := entities[prev]
			if ent.ObjectID == old.ObjectID || ent.UniqueID == old.UniqueID || ent.StateTopic == old.StateTopic || ent.DiscoveryTopic == old.DiscoveryTopic || reflect.DeepEqual(ent.Device["identifiers"], old.Device["identifiers"]) {
				t.Fatalf("bridge %d shares entity/device identity with %d", index, prev)
			}
		}
	}
	if entities[0].UniqueID != "omada_exporter_"+objectID(name, labels) || entities[0].Labels["bridge_id"] != "" || !reflect.DeepEqual(entities[0].Device["identifiers"], []string{"omada_device_aa_bb_cc_dd_ee_ff"}) {
		t.Fatal("legacy identity changed")
	}
	if labels["bridge_id"] != "" {
		t.Fatal("scope mutated the shared input labels")
	}
	apartment.publishMetricState(entities[1], 0, time.Now())
	var payload map[string]any
	if err := json.Unmarshal(broker.messages[entities[1].StateTopic], &payload); err != nil || payload["bridge_id"] != "slobidska44" {
		t.Fatalf("state scope missing: %#v (%v)", payload, err)
	}
	derived := apartment.newDerivedEntity("omada_vpn_tx_bits_per_second", "Rate", labels, publishContext{})
	if derived.Labels["bridge_id"] != "slobidska44" || !strings.HasPrefix(derived.StateTopic, "omada_exporter_slobidska44/") {
		t.Fatal("derived metric escaped bridge namespace")
	}

	// A single physical phone has a separate observation in each site and bridge.
	clientLabels := map[string]string{"mac": "aa:bb:cc:dd:ee:ff", "site_id": "same-site-id", "site": "Default"}
	first, _ := apartment.clientTracker("omada_client_signal_pct", clientLabels)
	clientLabels["site_id"] = "second-site-id"
	second, _ := apartment.clientTracker("omada_client_signal_pct", clientLabels)
	if first.StateTopic == second.StateTopic {
		t.Fatal("different sites share a tracker")
	}
	firstDevice := deviceInfo(apartment.client, "omada_client_device_tracker", first.Labels)
	secondDevice := deviceInfo(apartment.client, "omada_client_device_tracker", second.Labels)
	if reflect.DeepEqual(firstDevice["identifiers"], secondDevice["identifiers"]) {
		t.Fatal("different sites share an HA device")
	}
	for _, metric := range []string{"omada_client_signal_pct", "omada_device_cpu_percentage", "omada_vpn_status", "omada_site_clients"} {
		deviceLabels := map[string]string{"mac": "aa:bb:cc:dd:ee:ff", "device_mac": "11:22:33:44:55:66", "vpn_id": "same-vpn", "site_id": "same-site-id"}
		if metric == "omada_client_signal_pct" {
			delete(deviceLabels, "device_mac")
		}
		if metric == "omada_vpn_status" {
			delete(deviceLabels, "mac")
			delete(deviceLabels, "device_mac")
		}
		if metric == "omada_site_clients" {
			delete(deviceLabels, "mac")
			delete(deviceLabels, "device_mac")
			delete(deviceLabels, "vpn_id")
		}
		old := deviceInfo(legacy.client, metric, deviceLabels)
		next := deviceInfo(apartment.client, metric, deviceLabels)
		if reflect.DeepEqual(old["identifiers"], next["identifiers"]) {
			t.Fatalf("%s shares registry identity", metric)
		}
	}
}

func TestBridgeScopeCustomPrefixesAndClientIDs(t *testing.T) {
	p := scopeTestPublisher(t, "flat-2", &recordingMQTTClient{})
	p.client.Settings().MQTTTopicPrefix = "/custom/state/"
	p.client.Settings().MQTTClientID = "custom-client"
	p.client.Settings().MQTTDiscoveryPrefix = "ha/discovery"
	if p.statePrefix() != "custom/state_flat-2" || p.mqttClientOptions().ClientID != "custom-client_flat-2" || p.discoveryNode() != "omada_exporter_flat-2" {
		t.Fatal("custom settings are not deterministically scoped")
	}
	name := "omada_site_clients"
	ent := p.newMetricEntity(&dto.MetricFamily{Name: &name}, nil, publishContext{})
	if !strings.HasPrefix(ent.DiscoveryTopic, "ha/discovery/sensor/omada_exporter_flat-2/") {
		t.Fatal("discovery root changed instead of the publisher node")
	}
}

// filteringScopeBroker emulates retained wildcard delivery using a common
// broker store, so tests exercise the actual subscriptions instead of routing
// messages to a hand-picked publisher.
type filteringScopeBroker struct {
	*recordingMQTTClient
	subscriptions []string
}

func (b *filteringScopeBroker) Subscribe(filter string, _ byte, handler mqtt.MessageHandler) mqtt.Token {
	b.subscriptions = append(b.subscriptions, filter)
	for topic, body := range b.messages {
		if mqttScopeMatch(filter, topic) && len(body) > 0 {
			handler(b, retainedMessage{topic: topic, payload: body})
		}
	}
	return completedToken{}
}

func mqttScopeMatch(filter, topic string) bool {
	want, got := strings.Split(filter, "/"), strings.Split(topic, "/")
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != "+" && want[i] != got[i] {
			return false
		}
	}
	return true
}

func TestBridgeScopeRetainedRecoveryAndOfflineTransitionsAreIsolated(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := &filteringScopeBroker{recordingMQTTClient: &recordingMQTTClient{}}
		legacy := scopeTestPublisher(t, "", broker)
		apartment := scopeTestPublisher(t, "slobidska44", broker)
		name := "omada_device_cpu_percentage"
		labels := map[string]string{"site_id": "same-site-id", "device_mac": "11:22:33:44:55:66"}
		phone := map[string]string{"site_id": "same-site-id", "mac": "aa:bb:cc:dd:ee:ff", "name": "Phone"}
		for _, pub := range []*Publisher{legacy, apartment} {
			ent := pub.newMetricEntity(&dto.MetricFamily{Name: &name}, labels, publishContext{})
			pub.publishDiscovery(ent, dto.MetricType_GAUGE)
			pub.publishMetricState(ent, 12, time.Now())
			tracker, _ := pub.clientTracker("omada_client_signal_pct", phone)
			pub.publishClientTrackers(map[string]clientTracker{pub.clientTrackerID(phone): tracker})
		}
		for _, id := range []string{"", "slobidska44"} {
			restarted := scopeTestPublisher(t, id, broker)
			if err := restarted.loadRetainedInventory(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(restarted.retainedDiscovery) != 1 || len(restarted.retainedStates) != 1 || len(restarted.knownClients) != 1 {
				t.Fatalf("%q recovered foreign or missing records: discovery %d states %d clients %d", id, len(restarted.retainedDiscovery), len(restarted.retainedStates), len(restarted.knownClients))
			}
			for topic, attrs := range restarted.retainedStates {
				if !restarted.ownsEntityState(topic) || attrs["bridge_id"] != id {
					t.Fatal("foreign state imported")
				}
			}
			for _, tracker := range restarted.knownClients {
				if tracker.Labels["bridge_id"] != id || tracker.Labels["name"] != "Phone" {
					t.Fatal("tracker attributes not recovered")
				}
			}
			before := map[string]int{}
			for topic, count := range broker.publishCounts {
				before[topic] = count
			}
			restarted.trackedClientMACs = nil // dynamic restart recovery must also be isolated
			restarted.publishClientTrackers(nil)
			for topic, count := range broker.publishCounts {
				if count != before[topic] && !strings.HasPrefix(topic, restarted.statePrefix()+"/") {
					t.Fatalf("restart %q touched foreign %s", id, topic)
				}
			}
		}
	})
}

func TestBridgeScopeReconciliationAndInfrastructureCleanupCannotDeleteOtherBridge(t *testing.T) {
	broker := &recordingMQTTClient{}
	legacy := scopeTestPublisher(t, "", broker)
	apartment := scopeTestPublisher(t, "slobidska44", broker)
	name := "omada_device_cpu_percentage"
	labels := map[string]string{"site_id": "same-site-id", "site": "Default", "device_mac": "11:22:33:44:55:66"}
	for _, pub := range []*Publisher{legacy, apartment} {
		pub.retainedLoaded = true
		pub.trackedClientMACs = nil
		current := pub.newMetricEntity(&dto.MetricFamily{Name: &name}, labels, publishContext{})
		for _, oldOwner := range []*Publisher{legacy, apartment} {
			oldDiscovery := fmt.Sprintf("homeassistant/sensor/%s/old/config", oldOwner.discoveryNode())
			oldState := oldOwner.statePrefix() + "/entities/old/state"
			pub.retainedDiscovery[oldDiscovery] = oldState
			oldLabels := oldOwner.scopedLabels(labels)
			oldLabels["metric"] = name
			pub.retainedStates[oldState] = oldLabels
		}
		// Same site name but different ID must not be reconciled in scoped mode.
		otherSiteState := pub.statePrefix() + "/entities/other-site/state"
		otherSiteDiscovery := fmt.Sprintf("homeassistant/sensor/%s/other-site/config", pub.discoveryNode())
		otherSiteLabels := pub.scopedLabels(labels)
		otherSiteLabels["site_id"] = "another-site"
		otherSiteLabels["metric"] = name
		pub.retainedDiscovery[otherSiteDiscovery] = otherSiteState
		pub.retainedStates[otherSiteState] = otherSiteLabels
		broker.messages = map[string][]byte{}
		pub.reconcileSupersededEntities(map[string]entity{current.DiscoveryTopic: current})
		pub.removeInfrastructureClientTrackers(publishContext{infrastructureByMAC: map[string]map[string]string{"11_22_33_44_55_66": labels}})
		if len(broker.messages) < 5 {
			t.Fatal("expected own obsolete entity and duplicate tracker tombstones")
		}
		for topic := range broker.messages {
			if !strings.HasPrefix(topic, pub.statePrefix()+"/") && !pub.ownsDiscovery(topic) {
				t.Fatalf("cleanup escaped namespace: %s", topic)
			}
		}
		if pub.client.Settings().BridgeID != "" {
			if _, touched := broker.messages[otherSiteState]; touched {
				t.Fatal("same-name different site was deleted")
			}
		}
	}
}

func TestBridgeScopeGatherPublishesBridgeAttributesAndDerivedRates(t *testing.T) {
	broker := &recordingMQTTClient{}
	p := scopeTestPublisher(t, "slobidska44", broker)
	metric := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "omada_vpn_up_bytes", Help: "VPN traffic"}, []string{"site_id", "name", "interface_name", "vpn_mode", "vpn_type"})
	metric.WithLabelValues("same-site-id", "Tunnel", "VPN1", "client", "openvpn").Add(100)
	p.registry.MustRegister(metric)
	p.publishAll()
	metricStates := 0
	for topic, body := range broker.messages {
		if !p.ownsEntityState(topic) {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["bridge_id"] != "slobidska44" {
			t.Fatal("gather/derived state missing bridge identity")
		}
		metricStates++
	}
	if metricStates != 2 {
		t.Fatalf("got %d raw/derived states, want 2", metricStates)
	}
}

func TestBridgeScopeCanonicalRecoveryRejectsForeignIdentityAndStateTopics(t *testing.T) {
	p := scopeTestPublisher(t, "slobidska44", &recordingMQTTClient{})
	name := "omada_device_cpu_percentage"
	labels := map[string]string{"device_mac": "11:22:33:44:55:66", "site_id": "same-site-id"}
	want := p.newMetricEntity(&dto.MetricFamily{Name: &name}, labels, publishContext{})
	p.retainedLoaded = true
	for index, invalid := range []struct{ state, discovery, bridgeID, siteID string }{
		{"omada_exporter/entities/foreign/state", "homeassistant/sensor/omada_exporter_slobidska44/foreign/config", "slobidska44", "same-site-id"},
		{"omada_exporter_slobidska44/entities/foreign/state", "homeassistant/sensor/omada_exporter/foreign/config", "slobidska44", "same-site-id"},
		{"omada_exporter_slobidska44/entities/foreign/state", "homeassistant/sensor/omada_exporter_slobidska44/foreign/config", "other", "same-site-id"},
		{"omada_exporter_slobidska44/entities/foreign/state", "homeassistant/sensor/omada_exporter_slobidska44/foreign/config", "slobidska44", "another-site-id"},
	} {
		retainedLabels := copyLabels(labels)
		retainedLabels["metric"] = name
		retainedLabels["bridge_id"] = invalid.bridgeID
		retainedLabels["site_id"] = invalid.siteID
		p.retainedDiscovery = map[string]string{invalid.discovery: invalid.state}
		p.retainedStates = map[string]map[string]string{invalid.state: retainedLabels}
		got := p.newMetricEntity(&dto.MetricFamily{Name: &name}, labels, publishContext{})
		if got.ObjectID != want.ObjectID || got.DiscoveryTopic != want.DiscoveryTopic || got.StateTopic != want.StateTopic {
			t.Fatalf("case %d adopted foreign retained entity", index)
		}
	}
	labels["bridge_id"] = "another-bridge"
	got := p.newMetricEntity(&dto.MetricFamily{Name: &name}, labels, publishContext{})
	if got.Labels["bridge_id"] != "slobidska44" || got.ObjectID != want.ObjectID {
		t.Fatal("input labels overrode configured publisher identity")
	}
}
