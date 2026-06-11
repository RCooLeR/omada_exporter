package hamqtt

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
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
}

type recordingMQTTClient struct {
	messages map[string][]byte
}

func (c *recordingMQTTClient) IsConnected() bool { return true }

func (c *recordingMQTTClient) IsConnectionOpen() bool { return true }

func (c *recordingMQTTClient) Connect() mqtt.Token { return completedToken{} }

func (c *recordingMQTTClient) Disconnect(uint) {}

func (c *recordingMQTTClient) Publish(topic string, _ byte, _ bool, payload any) mqtt.Token {
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

func (c *recordingMQTTClient) Subscribe(string, byte, mqtt.MessageHandler) mqtt.Token {
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

type completedToken struct{}

func (completedToken) Wait() bool { return true }

func (completedToken) WaitTimeout(time.Duration) bool { return true }

func (completedToken) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func (completedToken) Error() error { return nil }
