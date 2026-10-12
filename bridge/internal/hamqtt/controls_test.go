package hamqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/webapi"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type controlTestAPI struct {
	mu                       sync.Mutex
	targets                  []webapi.ControlTarget
	inventoryErr, commandErr error
	calls                    []string
	block                    bool
	started                  chan struct{}
}

func (f *controlTestAPI) ControlTargets(ctx context.Context) ([]webapi.ControlTarget, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]webapi.ControlTarget(nil), f.targets...), f.inventoryErr
}
func (f *controlTestAPI) command(ctx context.Context, action, siteID, mac string) error {
	f.mu.Lock()
	f.calls = append(f.calls, action+":"+siteID+":"+mac)
	block, started, err := f.block, f.started, f.commandErr
	f.mu.Unlock()
	if started != nil {
		close(started)
	}
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}
func (f *controlTestAPI) RebootDevice(ctx context.Context, siteID, mac string) error {
	return f.command(ctx, controlReboot, siteID, mac)
}
func (f *controlTestAPI) ReconnectWiFiClient(ctx context.Context, siteID, mac string) error {
	return f.command(ctx, controlReconnect, siteID, mac)
}

type controlMessage struct {
	topic, payload      string
	retained, duplicate bool
	qos                 byte
}

func (m controlMessage) Duplicate() bool   { return m.duplicate }
func (m controlMessage) Qos() byte         { return m.qos }
func (m controlMessage) Retained() bool    { return m.retained }
func (m controlMessage) Topic() string     { return m.topic }
func (m controlMessage) MessageID() uint16 { return 0 }
func (m controlMessage) Payload() []byte   { return []byte(m.payload) }
func (m controlMessage) Ack()              {}

// Unlike the metric test fixture, this fake is safe for worker/callback races.
type controlTestBroker struct {
	*recordingMQTTClient
	mu            sync.Mutex
	handlers      map[string]mqtt.MessageHandler
	subscriptions []string
	subscribeErr  error
}

func newControlTestBroker() *controlTestBroker {
	return &controlTestBroker{recordingMQTTClient: &recordingMQTTClient{}, handlers: map[string]mqtt.MessageHandler{}}
}
func (b *controlTestBroker) Publish(topic string, qos byte, retained bool, payload any) mqtt.Token {
	b.mu.Lock()
	defer b.mu.Unlock()
	if qos != 0 {
		panic("control publications must use QoS 0")
	}
	if !retained {
		panic("only discovery/state publications should reach this fake")
	}
	if strings.HasSuffix(topic, "/command") {
		panic("bridge must never publish a command")
	}
	return b.recordingMQTTClient.Publish(topic, qos, retained, payload)
}
func (b *controlTestBroker) Subscribe(filter string, qos byte, handler mqtt.MessageHandler) mqtt.Token {
	b.mu.Lock()
	defer b.mu.Unlock()
	if qos != 0 {
		panic("control subscription must use QoS 0")
	}
	b.subscriptions = append(b.subscriptions, filter)
	b.handlers[filter] = handler
	return completedToken{err: b.subscribeErr}
}
func (b *controlTestBroker) payload(topic string) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.messages[topic]...)
}

func controlTargetsFixture() []webapi.ControlTarget {
	return []webapi.ControlTarget{
		{SiteID: "same-site-id", MAC: "AA-BB-CC-DD-EE-01", Name: "Access point", DeviceType: "ap", Model: "EAP", Action: webapi.ControlActionRebootDevice},
		{SiteID: "same-site-id", MAC: "AA-BB-CC-DD-EE-02", Name: "Phone", Action: webapi.ControlActionReconnectWiFiClient},
	}
}
func controlFixture(t *testing.T, bridge string) (*Publisher, *controlTestAPI, *controlTestBroker) {
	t.Helper()
	b := newControlTestBroker()
	p := scopeTestPublisher(t, bridge, b)
	p.client.Config.AllowDeviceControl = true
	f := &controlTestAPI{targets: controlTargetsFixture()}
	p.controls.api = f
	p.controls.ctx = context.Background()
	p.controls.connected = true
	p.refreshControls(context.Background())
	return p, f, b
}
func controlButtonFor(t *testing.T, p *Publisher, action string) controlButton {
	t.Helper()
	p.controls.mu.Lock()
	defer p.controls.mu.Unlock()
	for _, button := range p.controls.buttons {
		if button.Action == action {
			return button
		}
	}
	t.Fatalf("missing %s button", action)
	return controlButton{}
}
func requireNoControlCommand(t *testing.T, p *Publisher) {
	t.Helper()
	select {
	case command := <-p.controls.commands:
		t.Fatalf("unexpected queued command: %+v", command)
	default:
	}
}

func TestControlsDisabledByDefaultOnlyClearAvailability(t *testing.T) {
	b := newControlTestBroker()
	p := scopeTestPublisher(t, "slobidska44", b)
	stop := p.startControls(context.Background())
	defer stop()
	p.controlsConnected(b)
	if len(b.subscriptions) != 0 {
		t.Fatal("disabled controls subscribed to commands")
	}
	if len(b.messages) != 1 || string(b.payload(p.controlAvailabilityTopic())) != "offline" {
		t.Fatal("disabled controls must only clear their own availability")
	}
	if len(p.controls.buttons) != 0 {
		t.Fatal("disabled controls created buttons")
	}
}

func TestControlDiscoveryContractAndDeviceRegistryIdentity(t *testing.T) {
	for _, bridge := range []string{"", "slobidska44"} {
		t.Run(bridge, func(t *testing.T) {
			p, _, b := controlFixture(t, bridge)
			for _, action := range []string{controlReboot, controlReconnect} {
				button := controlButtonFor(t, p, action)
				var discovery, attrs map[string]any
				if err := json.Unmarshal(b.payload(button.DiscoveryTopic), &discovery); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(b.payload(button.AttributesTopic), &attrs); err != nil {
					t.Fatal(err)
				}
				if discovery["payload_press"] != "PRESS" || discovery["qos"] != float64(0) || discovery["retain"] != false || discovery["availability_mode"] != "all" {
					t.Fatalf("unsafe discovery: %#v", discovery)
				}
				if len(discovery["availability"].([]any)) != 3 {
					t.Fatal("missing bridge/global/target availability")
				}
				if attrs["omada_control"] != action || attrs["target_mac"] != button.MAC || attrs["site_id"] != "same-site-id" || attrs["site"] != "Default" {
					t.Fatalf("wrong attrs: %#v", attrs)
				}
				if _, exists := attrs["metric"]; exists {
					t.Fatal("control polluted metric contract")
				}
				if bridge == "" {
					if _, exists := attrs["bridge_id"]; exists {
						t.Fatal("legacy bridge must omit bridge_id")
					}
				} else if attrs["bridge_id"] != bridge {
					t.Fatal("missing bridge scope")
				}
				labels := p.scopedLabels(map[string]string{"site": "Default", "site_id": "same-site-id"})
				metric := "omada_device_status"
				if action == controlReboot {
					labels["device_mac"] = button.MAC
				} else {
					labels["mac"] = button.MAC
					metric = "omada_client_device_tracker"
				}
				want := deviceInfo(p.client, metric, labels)["identifiers"]
				if !reflect.DeepEqual(button.Config["device"].(map[string]any)["identifiers"], want) {
					t.Fatal("button created a separate device registry identity")
				}
			}
		})
	}
}

func TestControlTargetsRejectWrongSiteAndUnsupportedDeviceTypes(t *testing.T) {
	p, f, _ := controlFixture(t, "slobidska44")
	for _, kind := range []string{"controller", "olt", "stack", "unknown"} {
		f.targets = append(f.targets, webapi.ControlTarget{SiteID: "same-site-id", MAC: "AA-BB-CC-DD-EE-09", DeviceType: kind, Action: controlReboot})
	}
	f.targets = append(f.targets, webapi.ControlTarget{SiteID: "foreign-site", MAC: "AA-BB-CC-DD-EE-08", DeviceType: "ap", Action: controlReboot})
	p.refreshControls(context.Background())
	if len(p.controls.buttons) != 2 {
		t.Fatal("unsupported targets became controls")
	}
}

func TestControlAmbiguousInventoryTargetsAreNotAdvertised(t *testing.T) {
	p, f, _ := controlFixture(t, "slobidska44")
	f.targets = append(f.targets, f.targets[0], f.targets[0])
	p.refreshControls(context.Background())
	if len(p.controls.buttons) != 1 {
		t.Fatal("ambiguous reboot target was advertised")
	}
	controlButtonFor(t, p, controlReconnect)
}

func TestControlCommandsStrictEnvelope(t *testing.T) {
	cases := []struct {
		name   string
		change func(*controlMessage)
	}{
		{"retained", func(m *controlMessage) { m.retained = true }}, {"duplicate", func(m *controlMessage) { m.duplicate = true }},
		{"qos1", func(m *controlMessage) { m.qos = 1 }}, {"qos2", func(m *controlMessage) { m.qos = 2 }},
		{"lowercase", func(m *controlMessage) { m.payload = "press" }}, {"whitespace", func(m *controlMessage) { m.payload = "PRESS\n" }},
		{"json", func(m *controlMessage) { m.payload = `{"command":"PRESS"}` }}, {"oversized", func(m *controlMessage) { m.payload = strings.Repeat("X", 65536) }},
		{"unknown", func(m *controlMessage) { m.topic += "/extra" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, f, b := controlFixture(t, "slobidska44")
			button := controlButtonFor(t, p, controlReboot)
			message := controlMessage{topic: button.CommandTopic, payload: "PRESS"}
			tc.change(&message)
			p.handleControlMessage(b, message)
			requireNoControlCommand(t, p)
			if len(f.calls) != 0 {
				t.Fatal("invalid message called controller")
			}
		})
	}
}

func TestControlsSinglePendingCooldownAndNoMutationRetry(t *testing.T) {
	p, f, b := controlFixture(t, "slobidska44")
	reboot := controlButtonFor(t, p, controlReboot)
	reconnect := controlButtonFor(t, p, controlReconnect)
	p.handleControlMessage(b, controlMessage{topic: reboot.CommandTopic, payload: "PRESS"})
	for range 1000 {
		p.handleControlMessage(b, controlMessage{topic: reconnect.CommandTopic, payload: "PRESS"})
	}
	if len(p.controls.commands) != 1 {
		t.Fatal("command storm exceeded single pending bound")
	}
	p.executeControl(context.Background(), <-p.controls.commands)
	if len(f.calls) != 1 || !strings.HasPrefix(f.calls[0], controlReboot+":same-site-id:") {
		t.Fatalf("unexpected calls: %v", f.calls)
	}
	p.handleControlMessage(b, controlMessage{topic: reboot.CommandTopic, payload: "PRESS"})
	requireNoControlCommand(t, p)
	f.commandErr = errors.New("sensitive controller response must not be published")
	p.handleControlMessage(b, controlMessage{topic: reconnect.CommandTopic, payload: "PRESS"})
	p.executeControl(context.Background(), <-p.controls.commands)
	p.publishControlStatuses()
	if len(f.calls) != 2 {
		t.Fatal("mutation was retried")
	}
	attrs := string(b.payload(reconnect.AttributesTopic))
	if !strings.Contains(attrs, `"last_status":"failed"`) || strings.Contains(attrs, "sensitive") || !strings.Contains(attrs, "last_status_at") {
		t.Fatalf("bad failure attributes: %s", attrs)
	}
	if string(b.payload(p.controlAvailabilityTopic())) != "offline" {
		t.Fatal("failed command did not fail closed")
	}
}

func TestControlCooldownSurvivesInventoryFlap(t *testing.T) {
	p, f, b := controlFixture(t, "")
	button := controlButtonFor(t, p, controlReboot)
	p.handleControlMessage(b, controlMessage{topic: button.CommandTopic, payload: "PRESS"})
	p.executeControl(context.Background(), <-p.controls.commands)
	original := f.targets
	f.targets = nil
	p.refreshControls(context.Background())
	f.targets = original
	p.refreshControls(context.Background())
	p.handleControlMessage(b, controlMessage{topic: button.CommandTopic, payload: "PRESS"})
	requireNoControlCommand(t, p)
	p.controls.lastAttempt[button.CommandTopic] = time.Now().Add(-controlCooldown)
	p.handleControlMessage(b, controlMessage{topic: button.CommandTopic, payload: "PRESS"})
	if len(p.controls.commands) != 1 {
		t.Fatal("cooldown never expired")
	}
}

func TestControlInventoryAndDiscoveryFailureFailClosed(t *testing.T) {
	for _, mode := range []string{"inventory", "publish", "stale", "disabled", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			p, f, b := controlFixture(t, "")
			button := controlButtonFor(t, p, controlReboot)
			switch mode {
			case "inventory":
				f.inventoryErr = errors.New("inventory unavailable")
				p.refreshControls(context.Background())
			case "publish":
				b.publishFailures = map[string]int{button.DiscoveryTopic: 1}
				p.refreshControls(context.Background())
			case "stale":
				p.controls.validatedAt = time.Now().Add(-controlInventoryMaxAge - time.Second)
			case "disabled":
				p.client.Config.AllowDeviceControl = false
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				p.controls.ctx = ctx
			}
			p.handleControlMessage(b, controlMessage{topic: button.CommandTopic, payload: "PRESS"})
			requireNoControlCommand(t, p)
			if len(f.calls) != 0 {
				t.Fatal("unavailable control executed")
			}
		})
	}
}

func TestControlBridgeAndSiteIsolation(t *testing.T) {
	legacy, _, _ := controlFixture(t, "")
	apartment, _, b := controlFixture(t, "slobidska44")
	old := controlButtonFor(t, legacy, controlReboot)
	current := controlButtonFor(t, apartment, controlReboot)
	if old.CommandTopic == current.CommandTopic || old.DiscoveryTopic == current.DiscoveryTopic {
		t.Fatal("control namespaces collide")
	}
	apartment.handleControlMessage(b, controlMessage{topic: old.CommandTopic, payload: "PRESS"})
	requireNoControlCommand(t, apartment)
	apartment.client.SetContextIDs("same-controller-id", "new-site")
	apartment.handleControlMessage(b, controlMessage{topic: current.CommandTopic, payload: "PRESS"})
	requireNoControlCommand(t, apartment)
	api := apartment.controls.api.(*controlTestAPI)
	for i := range api.targets {
		api.targets[i].SiteID = "new-site"
	}
	apartment.refreshControls(context.Background())
	updated := controlButtonFor(t, apartment, controlReboot)
	if updated.CommandTopic == current.CommandTopic {
		t.Fatal("site change reused command topic")
	}
	apartment.handleControlMessage(b, controlMessage{topic: updated.CommandTopic, payload: "PRESS"})
	if len(apartment.controls.commands) != 1 {
		t.Fatal("new current site cannot receive commands")
	}
}

func TestControlQueuedCommandCannotCrossReconnectOrExpire(t *testing.T) {
	for _, mode := range []string{"reconnect", "expired", "site-change", "removed"} {
		t.Run(mode, func(t *testing.T) {
			p, f, b := controlFixture(t, "slobidska44")
			button := controlButtonFor(t, p, controlReboot)
			p.handleControlMessage(b, controlMessage{topic: button.CommandTopic, payload: "PRESS"})
			command := <-p.controls.commands
			switch mode {
			case "reconnect":
				p.controlsDisconnected()
				p.controlsConnected(b)
				p.refreshControls(context.Background())
			case "expired":
				command.ReceivedAt = time.Now().Add(-controlCommandMaxAge - time.Second)
			case "site-change":
				p.client.SetContextIDs("same-controller-id", "different-site")
			case "removed":
				f.targets = nil
				p.refreshControls(context.Background())
			}
			p.executeControl(context.Background(), command)
			if len(f.calls) != 0 || p.controls.pending {
				t.Fatal("stale queued command executed or blocked all controls forever")
			}
		})
	}
}

func TestControlRetainedRestartCleanupIsStrictlyOwned(t *testing.T) {
	old, _, _ := controlFixture(t, "slobidska44")
	foreign, _, _ := controlFixture(t, "")
	oldButton := controlButtonFor(t, old, controlReconnect)
	foreignButton := controlButtonFor(t, foreign, controlReconnect)
	p, f, b := controlFixture(t, "slobidska44")
	f.targets = nil
	for _, button := range []controlButton{oldButton, foreignButton} {
		body, _ := json.Marshal(button.Config)
		p.handleRetainedControl(b, controlMessage{topic: button.DiscoveryTopic, payload: string(body), retained: true})
	}
	if len(p.controls.retained) != 1 {
		t.Fatal("retained discovery admitted foreign namespace or missed own button")
	}
	p.refreshControls(context.Background())
	if payload, exists := b.messages[oldButton.DiscoveryTopic]; !exists || len(payload) != 0 {
		t.Fatal("removed own button discovery was not cleared")
	}
	if _, exists := b.messages[foreignButton.DiscoveryTopic]; exists {
		t.Fatal("foreign discovery was modified")
	}
	if string(b.payload(oldButton.AvailabilityTopic)) != "offline" {
		t.Fatal("removed target still available")
	}
	// A retained command can never make a target eligible after startup.
	p.handleControlMessage(b, controlMessage{topic: oldButton.CommandTopic, payload: "PRESS", retained: true})
	requireNoControlCommand(t, p)
}

func TestControlReconnectResubscribesAndSubscriptionFailureStaysOffline(t *testing.T) {
	p, _, b := controlFixture(t, "slobidska44")
	p.controlsDisconnected()
	p.controlsConnected(b)
	if p.controls.ready {
		t.Fatal("reconnect reused previous eligibility")
	}
	if len(b.subscriptions) != 2 {
		t.Fatal("did not resubscribe commands and retained inventory")
	}
	p.refreshControls(context.Background())
	p.controlsDisconnected()
	p.controlsConnected(b)
	if len(b.subscriptions) != 4 || p.controls.ready {
		t.Fatal("second reconnect did not fail closed and resubscribe")
	}
	b.subscribeErr = errors.New("broker denied subscription")
	p.controlsDisconnected()
	p.controlsConnected(b)
	p.refreshControls(context.Background())
	if p.controls.ready || p.controls.connected || string(b.payload(p.controlAvailabilityTopic())) != "offline" {
		t.Fatal("failed subscription allowed controls")
	}
}

func TestControlWorkerShutdownCancelsInFlightRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, f, b := controlFixture(t, "slobidska44")
		f.block = true
		f.started = make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		stop := p.startControls(ctx)
		button := controlButtonFor(t, p, controlReboot)
		p.handleControlMessage(b, controlMessage{topic: button.CommandTopic, payload: "PRESS"})
		<-f.started
		cancel()
		stop()
		f.mu.Lock()
		count := len(f.calls)
		f.mu.Unlock()
		if count != 1 || string(b.payload(p.controlAvailabilityTopic())) != "offline" {
			t.Fatal("shutdown did not cancel and disable controls")
		}
	})
}

func TestControlRetainedDiscoveryCannotSpoofOtherStateTopics(t *testing.T) {
	p, _, b := controlFixture(t, "slobidska44")
	button := controlButtonFor(t, p, controlReboot)
	for _, mutation := range []string{"attributes", "discovery", "command", "oversized"} {
		config := map[string]any{}
		for key, value := range button.Config {
			config[key] = value
		}
		topic := button.DiscoveryTopic
		switch mutation {
		case "attributes":
			config["json_attributes_topic"] = "foreign/state"
		case "discovery":
			topic = "homeassistant/sensor/foreign/test/config"
		case "command":
			config["command_topic"] = button.CommandTopic + "/extra"
		}
		body, _ := json.Marshal(config)
		if mutation == "oversized" {
			body = []byte(fmt.Sprintf(`{"padding":%q}`, strings.Repeat("x", 16384)))
		}
		p.handleRetainedControl(b, controlMessage{topic: topic, payload: string(body), retained: true})
	}
	if len(p.controls.retained) != 0 {
		t.Fatal("spoofed retained control entered cleanup inventory")
	}
}
