package hamqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

type poeCommandCall struct {
	siteID, ownerMAC, poweredDeviceMAC string
	port                               int
}

type poeControlTestAPI struct {
	*controlTestAPI
	poeTargets          []omada.ControlTarget
	poeInventoryErr     error
	poeCalls            []poeCommandCall
	poeReadHasDeadline  bool
	poeWriteHasDeadline bool
	poeReads            int
	poeReadStarted      chan struct{}
	poeReadCallback     func()
	poeReadBlock        bool
}

func (f *poeControlTestAPI) PoEControlTargets(ctx context.Context) ([]omada.ControlTarget, error) {
	f.mu.Lock()
	_, f.poeReadHasDeadline = ctx.Deadline()
	f.poeReads++
	targets, err := append([]omada.ControlTarget(nil), f.poeTargets...), f.poeInventoryErr
	started, callback, block := f.poeReadStarted, f.poeReadCallback, f.poeReadBlock
	f.mu.Unlock()
	if started != nil {
		close(started)
	}
	if callback != nil {
		callback()
	}
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return targets, err
}

func (f *poeControlTestAPI) PowerCyclePoEPort(ctx context.Context, siteID, ownerMAC string, port int, expectedPoweredDeviceMAC string) error {
	f.mu.Lock()
	_, f.poeWriteHasDeadline = ctx.Deadline()
	f.poeCalls = append(f.poeCalls, poeCommandCall{siteID, ownerMAC, expectedPoweredDeviceMAC, port})
	f.mu.Unlock()
	return f.command(ctx, omada.ControlActionPowerCyclePoE, siteID, ownerMAC)
}

func poeTargetFixture() omada.ControlTarget {
	return omada.ControlTarget{
		SiteID: "same-site-id", MAC: "02-00-00-00-00-10", Name: "Edge switch", Model: "Switch", DeviceType: "switch",
		Action: omada.ControlActionPowerCyclePoE, Port: 4, PortName: "AP port",
		PoweredDeviceMAC: "02:00:00:00:00:20", PoweredDeviceName: "Access point",
	}
}

func poeControlFixture(t *testing.T, bridge string) (*Publisher, *poeControlTestAPI, *controlTestBroker) {
	t.Helper()
	p, base, broker := controlFixture(t, bridge)
	provider := &poeControlTestAPI{controlTestAPI: base, poeTargets: []omada.ControlTarget{poeTargetFixture()}}
	p.controls.api = provider
	p.refreshControls(t.Context())
	return p, provider, broker
}

func TestPoEControlDiscoveryUsesOwnerPortAndBinding(t *testing.T) {
	for _, bridge := range []string{"", "slobidska44"} {
		for _, ownerType := range []string{"switch", "gateway"} {
			t.Run(bridge+"/"+ownerType, func(t *testing.T) {
				p, provider, broker := poeControlFixture(t, bridge)
				provider.poeTargets[0].DeviceType = ownerType
				p.refreshControls(t.Context())
				button := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
				if !strings.HasSuffix(button.CommandTopic, "/power_cycle_poe/02_00_00_00_00_10_p4_b02_00_00_00_00_20/command") {
					t.Fatalf("unexpected port topic %s", button.CommandTopic)
				}
				if strings.Count(strings.TrimPrefix(button.CommandTopic, p.statePrefix()+"/controls/"), "/") != 3 {
					t.Fatal("PoE changed the control topic depth")
				}
				var discovery, attributes map[string]any
				if err := json.Unmarshal(broker.payload(button.DiscoveryTopic), &discovery); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(broker.payload(button.AttributesTopic), &attributes); err != nil {
					t.Fatal(err)
				}
				if discovery["payload_press"] != "PRESS:02-00-00-00-00-20" || discovery["qos"] != float64(0) || discovery["retain"] != false {
					t.Fatalf("unsafe PoE discovery: %v", discovery)
				}
				for key, expected := range map[string]any{"target_port": float64(4), "switch_name": "Edge switch", "port_name": "AP port", "powered_device_mac": "02-00-00-00-00-20", "powered_device_name": "Access point"} {
					if attributes[key] != expected {
						t.Errorf("%s = %v, want %v", key, attributes[key], expected)
					}
				}
				labels := p.scopedLabels(map[string]string{"site": "Default", "site_id": "same-site-id", "device_mac": button.MAC})
				want := deviceInfo(p.client, "omada_device_status", labels)["identifiers"]
				if !reflect.DeepEqual(button.Config["device"].(map[string]any)["identifiers"], want) {
					t.Fatal("PoE button is not grouped under its owner")
				}
				if !provider.poeReadHasDeadline {
					t.Fatal("PoE inventory has no deadline")
				}
			})
		}
	}
}

func TestPoEControlPortsRemainDistinctAndLegacyIDsStayUnchanged(t *testing.T) {
	p, provider, _ := poeControlFixture(t, "slobidska44")
	legacy := controlButtonFor(t, p, controlReboot)
	wantID := "omada_control_" + p.controlSiteKey() + "_reboot_device_aa_bb_cc_dd_ee_01"
	if legacy.Config["object_id"] != wantID || legacy.Payload != "PRESS" || !strings.HasSuffix(legacy.CommandTopic, "/reboot_device/aa_bb_cc_dd_ee_01/command") {
		t.Fatal("legacy identity or payload changed")
	}
	second := provider.poeTargets[0]
	second.Port, second.PoweredDeviceMAC, second.PoweredDeviceName = 5, "", ""
	provider.poeTargets = append(provider.poeTargets, second)
	p.refreshControls(t.Context())
	if len(p.controls.buttons) != 4 {
		t.Fatalf("buttons = %d, want two legacy and two PoE ports", len(p.controls.buttons))
	}
	ids := map[string]bool{}
	for _, button := range p.controls.buttons {
		id := button.Config["object_id"].(string)
		if ids[id] {
			t.Fatal("port identities collide")
		}
		ids[id] = true
		if button.Port == 5 && (button.Payload != "PRESS:" || button.Attributes["powered_device_mac"] != nil || button.Attributes["powered_device_name"] != nil) {
			t.Fatal("unbound PoE port fabricated powered-device identity")
		}
	}
}

func TestPoEControlRequiresOptionalCapabilityAndValidTargets(t *testing.T) {
	p, base, _ := controlFixture(t, "")
	base.targets = append(base.targets, poeTargetFixture())
	p.refreshControls(t.Context())
	if len(p.controls.buttons) != 2 {
		t.Fatal("legacy API advertised unsupported PoE capability")
	}
	p, _, _ = poeControlFixture(t, "")
	for _, change := range []func(*omada.ControlTarget){
		func(target *omada.ControlTarget) { target.Port = 0 },
		func(target *omada.ControlTarget) { target.Port = -1 },
		func(target *omada.ControlTarget) { target.Port = 256 },
		func(target *omada.ControlTarget) { target.DeviceType = "ap" },
		func(target *omada.ControlTarget) { target.DeviceType = "controller" },
		func(target *omada.ControlTarget) { target.SiteID = "foreign-site" },
		func(target *omada.ControlTarget) { target.MAC = " 02-00-00-00-00-10" },
		func(target *omada.ControlTarget) { target.PoweredDeviceMAC = "not-a-mac" },
	} {
		target := poeTargetFixture()
		change(&target)
		if _, ok := p.makeControlButton(target); ok {
			t.Fatalf("invalid PoE target accepted: %+v", target)
		}
	}
}

func TestPoEInventoryFailureWithdrawsOnlyPoEButtons(t *testing.T) {
	p, provider, broker := poeControlFixture(t, "slobidska44")
	poe := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
	provider.poeInventoryErr = errors.New("optional endpoint unavailable")
	p.refreshControls(t.Context())
	if !p.controls.ready || len(p.controls.buttons) != 2 || len(broker.payload(poe.DiscoveryTopic)) != 0 || string(broker.payload(poe.AvailabilityTopic)) != "offline" {
		t.Fatal("optional PoE failure disabled legacy controls or left stale PoE discovery")
	}
	legacy := controlButtonFor(t, p, controlReboot)
	p.handleControlMessage(broker, controlMessage{topic: legacy.CommandTopic, payload: "PRESS"})
	p.executeControl(t.Context(), <-p.controls.commands)
	if len(provider.calls) != 1 || len(provider.poeCalls) != 0 {
		t.Fatal("legacy control unavailable after optional PoE failure")
	}
}

func TestPoEControlRejectsUnsafeMessageEnvelopesAndPayloads(t *testing.T) {
	for _, mode := range []string{"retained", "duplicate", "qos1", "qos2", "generic", "wrong-binding", "empty-binding", "lowercase", "whitespace", "oversized", "unknown", "stale-inventory"} {
		t.Run(mode, func(t *testing.T) {
			p, provider, broker := poeControlFixture(t, "")
			button := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
			message := controlMessage{topic: button.CommandTopic, payload: button.Payload}
			switch mode {
			case "retained":
				message.retained = true
			case "duplicate":
				message.duplicate = true
			case "qos1":
				message.qos = 1
			case "qos2":
				message.qos = 2
			case "generic":
				message.payload = "PRESS"
			case "wrong-binding":
				message.payload = "PRESS:02-00-00-00-00-99"
			case "empty-binding":
				message.payload = "PRESS:"
			case "lowercase":
				message.payload = strings.ToLower(message.payload)
			case "whitespace":
				message.payload += "\n"
			case "oversized":
				message.payload = strings.Repeat("X", 65536)
			case "unknown":
				message.topic += "/extra"
			case "stale-inventory":
				p.controls.validatedAt = time.Now().Add(-controlInventoryMaxAge - time.Second)
			}
			p.handleControlMessage(broker, message)
			requireNoControlCommand(t, p)
			if len(provider.calls) != 0 {
				t.Fatal("unsafe PoE message called controller")
			}
		})
	}
}

func TestPoEControlStaleBindingCannotAdmitOrExecute(t *testing.T) {
	for _, originalBinding := range []string{"", "02-00-00-00-00-20"} {
		for _, stage := range []string{"before-admission", "after-queue"} {
			t.Run(originalBinding+"/"+stage, func(t *testing.T) {
				p, provider, broker := poeControlFixture(t, "slobidska44")
				provider.poeTargets[0].PoweredDeviceMAC = originalBinding
				p.refreshControls(t.Context())
				old := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
				var command controlCommand
				if stage == "after-queue" {
					// Model a callback admitting the old target while an inventory
					// refresh is in flight, before the changed binding is applied.
					provider.poeReadCallback = func() {
						p.handleControlMessage(broker, controlMessage{topic: old.CommandTopic, payload: old.Payload})
						command = <-p.controls.commands
					}
				}
				provider.poeTargets[0].PoweredDeviceMAC = "02-00-00-00-00-30"
				p.refreshControls(t.Context())
				current := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
				if current.CommandTopic == old.CommandTopic || current.DiscoveryTopic == old.DiscoveryTopic || current.Config["unique_id"] == old.Config["unique_id"] || current.Payload == old.Payload {
					t.Fatal("port identity or binding payload incorrect")
				}
				if len(broker.payload(old.DiscoveryTopic)) != 0 || string(broker.payload(old.AvailabilityTopic)) != "offline" {
					t.Fatal("old bound HA entity remains available")
				}
				if stage == "after-queue" {
					p.executeControl(t.Context(), command)
				} else {
					p.handleControlMessage(broker, controlMessage{topic: old.CommandTopic, payload: old.Payload})
					requireNoControlCommand(t, p)
				}
				if len(provider.poeCalls) != 0 || p.controls.pending {
					t.Fatal("stale binding executed or blocked future controls")
				}
			})
		}
	}
}

func TestPoEControlExecutionUsesCapturedBindingWithoutRetry(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			p, provider, broker := poeControlFixture(t, "")
			button := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
			if failure {
				provider.commandErr = errors.New("request timeout")
			}
			p.handleControlMessage(broker, controlMessage{topic: button.CommandTopic, payload: button.Payload})
			command := <-p.controls.commands
			if command.PoweredDeviceMAC != "02-00-00-00-00-20" {
				t.Fatal("queue did not snapshot powered-device binding")
			}
			p.executeControl(t.Context(), command)
			want := []poeCommandCall{{"same-site-id", "02:00:00:00:00:10", "02-00-00-00-00-20", 4}}
			if !reflect.DeepEqual(provider.poeCalls, want) || !provider.poeWriteHasDeadline {
				t.Fatalf("PoE calls = %v", provider.poeCalls)
			}
			p.handleControlMessage(broker, controlMessage{topic: button.CommandTopic, payload: button.Payload})
			requireNoControlCommand(t, p)
			if len(provider.calls) != 1 {
				t.Fatal("PoE mutation retried or fell through to another action")
			}
		})
	}
}

func TestPoEControlScopeAndQueueFreshness(t *testing.T) {
	legacy, _, _ := poeControlFixture(t, "")
	foreign := controlButtonFor(t, legacy, omada.ControlActionPowerCyclePoE)
	for _, mode := range []string{"foreign-bridge", "site-change", "reconnect", "expired", "removed"} {
		t.Run(mode, func(t *testing.T) {
			p, provider, broker := poeControlFixture(t, "slobidska44")
			button := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
			if mode == "foreign-bridge" {
				p.handleControlMessage(broker, controlMessage{topic: foreign.CommandTopic, payload: foreign.Payload})
				requireNoControlCommand(t, p)
				return
			}
			p.handleControlMessage(broker, controlMessage{topic: button.CommandTopic, payload: button.Payload})
			command := <-p.controls.commands
			switch mode {
			case "site-change":
				p.client.(*session.Client).SetContextIDs("same-controller-id", "new-site")
			case "reconnect":
				p.controlsDisconnected()
				p.controlsConnected(broker)
				p.refreshControls(t.Context())
			case "expired":
				command.ReceivedAt = time.Now().Add(-controlCommandMaxAge - time.Second)
			case "removed":
				provider.poeTargets = nil
				p.refreshControls(t.Context())
			}
			p.executeControl(t.Context(), command)
			if len(provider.poeCalls) != 0 || p.controls.pending {
				t.Fatal("stale PoE command executed")
			}
		})
	}
}

func TestPoERetainedCleanupAcceptsOnlyExactOwnedPortTopics(t *testing.T) {
	p, provider, broker := poeControlFixture(t, "slobidska44")
	button := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
	for _, leaf := range []string{"02_00_00_00_00_10", "02_00_00_00_00_10_p4", "02_00_00_00_00_10_p0_bunknown", "02_00_00_00_00_10_p256_bunknown", "02_00_00_00_00_10_p04_bunknown", "02_00_00_00_00_10_p+4_bunknown", "02_00_00_00_00_10_p-1_bunknown", "02_00_00_00_00_10_p4_p5_bunknown", "02:00:00:00:00:10_p4_bunknown", "02_00_00_00_00_1x_p4_bunknown", "02_00_00_00_00_10_p4_bunknown/extra", "02_00_00_00_00_10_p4_b", "02_00_00_00_00_10_p4_bUNKNOWN", "02_00_00_00_00_10_p4_b02-00-00-00-00-20", "02_00_00_00_00_10_p4_b02_00_00_00_00_20_bunknown"} {
		config := map[string]any{}
		for key, value := range button.Config {
			config[key] = value
		}
		config["command_topic"] = p.statePrefix() + "/controls/" + p.controlSiteKey() + "/power_cycle_poe/" + leaf + "/command"
		body, _ := json.Marshal(config)
		p.handleRetainedControl(broker, controlMessage{topic: button.DiscoveryTopic, payload: string(body), retained: true})
	}
	if len(p.controls.retained) != 0 {
		t.Fatal("malformed PoE retained topic entered cleanup inventory")
	}
	foreign, _, _ := poeControlFixture(t, "")
	foreignButton := controlButtonFor(t, foreign, omada.ControlActionPowerCyclePoE)
	for _, entry := range []controlButton{button, foreignButton} {
		body, _ := json.Marshal(entry.Config)
		p.handleRetainedControl(broker, controlMessage{topic: entry.DiscoveryTopic, payload: string(body), retained: true})
	}
	if len(p.controls.retained) != 1 {
		t.Fatal("retained cleanup failed namespace ownership")
	}
	provider.poeTargets = nil
	p.refreshControls(t.Context())
	if len(broker.payload(button.DiscoveryTopic)) != 0 || string(broker.payload(button.AvailabilityTopic)) != "offline" {
		t.Fatal("retired PoE button was not cleared")
	}
	if _, touched := broker.messages[foreignButton.DiscoveryTopic]; touched {
		t.Fatal("foreign PoE discovery modified")
	}
	p.handleControlMessage(broker, controlMessage{topic: button.CommandTopic, payload: button.Payload, retained: true})
	requireNoControlCommand(t, p)
}

func TestPoEControlBindingChangesCannotResetPhysicalPortCooldown(t *testing.T) {
	p, provider, broker := poeControlFixture(t, "")
	old := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
	p.handleControlMessage(broker, controlMessage{topic: old.CommandTopic, payload: old.Payload})
	p.executeControl(t.Context(), <-p.controls.commands)
	provider.poeTargets[0].PoweredDeviceMAC = "02-00-00-00-00-30"
	p.refreshControls(t.Context())
	current := controlButtonFor(t, p, omada.ControlActionPowerCyclePoE)
	if current.CommandTopic == old.CommandTopic || controlCooldownKey(current) != controlCooldownKey(old) {
		t.Fatal("binding changed physical cooldown identity")
	}
	p.handleControlMessage(broker, controlMessage{topic: current.CommandTopic, payload: current.Payload})
	requireNoControlCommand(t, p)
	if len(provider.poeCalls) != 1 {
		t.Fatal("binding change bypassed port cooldown")
	}
}

func TestPoEControlAmbiguousPhysicalPortIsOmitted(t *testing.T) {
	p, provider, _ := poeControlFixture(t, "")
	conflicting := provider.poeTargets[0]
	conflicting.PoweredDeviceMAC = "02-00-00-00-00-30"
	provider.poeTargets = append(provider.poeTargets, conflicting)
	p.refreshControls(t.Context())
	if !p.controls.ready || len(p.controls.buttons) != 2 {
		t.Fatal("ambiguous PoE owner/port advertised multiple occupants or disabled legacy controls")
	}
}

func TestPoEOptionalInventoryCannotStarveQueuedLegacyControl(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, provider, broker := poeControlFixture(t, "")
		legacy := controlButtonFor(t, p, controlReboot)
		provider.poeReadStarted = make(chan struct{})
		provider.poeReadBlock = true
		refreshed := make(chan struct{})
		go func() {
			p.refreshControls(t.Context())
			close(refreshed)
		}()
		<-provider.poeReadStarted
		p.handleControlMessage(broker, controlMessage{topic: legacy.CommandTopic, payload: legacy.Payload})
		command := <-p.controls.commands
		<-refreshed
		if age := time.Since(command.ReceivedAt); age != poeControlInventoryTimeout || age >= controlCommandMaxAge {
			t.Fatalf("optional inventory delayed command by %s", age)
		}
		if !p.controls.ready || len(p.controls.buttons) != 2 {
			t.Fatal("slow optional inventory disabled successful legacy controls")
		}
		p.executeControl(t.Context(), command)
		if len(provider.calls) != 1 || len(provider.poeCalls) != 0 {
			t.Fatal("legacy command expired or was misrouted after optional inventory timeout")
		}
	})
}

func TestPoEOptionalInventoryIsSkippedForAlreadyPendingCommand(t *testing.T) {
	p, provider, broker := poeControlFixture(t, "")
	legacy := controlButtonFor(t, p, controlReboot)
	p.handleControlMessage(broker, controlMessage{topic: legacy.CommandTopic, payload: legacy.Payload})
	command := <-p.controls.commands
	reads := provider.poeReads
	p.refreshControls(t.Context())
	if provider.poeReads != reads {
		t.Fatal("optional PoE read delayed an already queued command")
	}
	p.executeControl(t.Context(), command)
	if len(provider.calls) != 1 {
		t.Fatal("queued legacy command did not execute")
	}
}
