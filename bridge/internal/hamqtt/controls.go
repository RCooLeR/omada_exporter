package hamqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/webapi"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	controlReboot          = "reboot_device"
	controlReconnect       = "reconnect_client"
	controlRefreshInterval = 60 * time.Second
	controlCooldown        = 60 * time.Second
	controlInventoryMaxAge = 2 * controlRefreshInterval
	controlCommandMaxAge   = 15 * time.Second
)

// The single worker owns controller requests and MQTT publications. Callbacks
// only update bounded state or enqueue one command; no goroutine is started per
// click. The controller API independently revalidates each mutation's target.
type controlAPI interface {
	ControlTargets(context.Context) ([]webapi.ControlTarget, error)
	RebootDevice(context.Context, string, string) error
	ReconnectWiFiClient(context.Context, string, string) error
}

type controlButton struct {
	Action, MAC, SiteID                                              string
	CommandTopic, DiscoveryTopic, AttributesTopic, AvailabilityTopic string
	Config                                                           map[string]any
	Attributes                                                       map[string]any
}

type controlCommand struct {
	Topic      string
	Epoch      uint64
	ReceivedAt time.Time
}

type controlState struct {
	mu                         sync.Mutex
	api                        controlAPI
	ctx                        context.Context
	ready, connected, pending  bool
	epoch                      uint64
	validatedAt                time.Time
	buttons                    map[string]controlButton // keyed by exact command topic
	retained                   map[string]controlButton // keyed by exact discovery topic
	lastAttempt                map[string]time.Time
	statusDirty                map[string]bool
	commands                   chan controlCommand
	refresh, reconcile, status chan struct{}
}

func newControlState() *controlState {
	return &controlState{
		buttons: map[string]controlButton{}, retained: map[string]controlButton{},
		lastAttempt: map[string]time.Time{}, statusDirty: map[string]bool{},
		commands: make(chan controlCommand, 1), refresh: make(chan struct{}, 1),
		reconcile: make(chan struct{}, 1), status: make(chan struct{}, 1),
	}
}

func notifyControl(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (p *Publisher) controlAvailabilityTopic() string { return p.statePrefix() + "/controls/status" }

func (p *Publisher) controlSiteKey() string {
	_, siteID := p.client.ContextIDs()
	return controlSiteKey(siteID, p.client.Config.Site)
}

func controlSiteKey(siteID, site string) string {
	return shortHash([]string{"site_id:" + siteID, "site:" + site})
}

func (p *Publisher) controlCommandFilter() string {
	// Only this bridge's namespace is subscribed. The live target map and
	// current site are checked again before admitting and executing a command.
	return fmt.Sprintf("%s/controls/+/+/+/command", p.statePrefix())
}

func (p *Publisher) controlDiscoveryFilter() string {
	return fmt.Sprintf("%s/button/%s/+/config", topicPrefix(p.client.Config.MQTTDiscoveryPrefix), p.discoveryNode())
}

func (p *Publisher) startControls(parent context.Context) func() {
	if p.controls == nil {
		p.controls = newControlState()
	}
	c := p.controls
	ctx, cancel := context.WithCancel(parent)
	c.mu.Lock()
	c.ctx = ctx
	if c.api == nil {
		c.api = &webapi.Client{Client: p.client}
	}
	c.mu.Unlock()
	done := make(chan struct{})
	if p.client.Config.AllowDeviceControl {
		go func() { defer close(done); p.runControls(ctx) }()
	} else {
		close(done)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			p.controlsDisconnected()
			<-done
			p.publishBytes(p.controlAvailabilityTopic(), []byte("offline"), true)
		})
	}
}

func (p *Publisher) controlsDisconnected() {
	if p.controls == nil {
		return
	}
	c := p.controls
	c.mu.Lock()
	c.ready, c.connected = false, false
	c.epoch++
	c.mu.Unlock()
}

func (p *Publisher) controlsConnected(client mqtt.Client) {
	var epoch uint64
	var ctx context.Context
	if p.controls != nil {
		c := p.controls
		c.mu.Lock()
		c.ready, c.connected = false, false
		c.epoch++
		epoch, ctx = c.epoch, c.ctx
		c.mu.Unlock()
	}
	// This retained fail-closed marker is intentional even when controls are
	// disabled: buttons from a formerly enabled process must not stay available.
	p.publishBytes(p.controlAvailabilityTopic(), []byte("offline"), true)
	if p.controls == nil || !p.client.Config.AllowDeviceControl {
		return
	}
	c := p.controls
	if ctx == nil || ctx.Err() != nil {
		return
	}
	for filter, handler := range map[string]mqtt.MessageHandler{
		p.controlDiscoveryFilter(): p.handleRetainedControl,
		p.controlCommandFilter():   p.handleControlMessage,
	} {
		requestCtx, cancel := context.WithTimeout(ctx, mqttConnectTimeout)
		err := waitForMQTTToken(requestCtx, client.Subscribe(filter, 0, handler))
		cancel()
		if err != nil {
			return
		}
	}
	c.mu.Lock()
	if c.epoch == epoch && ctx.Err() == nil {
		c.connected = true
	}
	c.mu.Unlock()
	notifyControl(c.refresh)
}

func (p *Publisher) makeControlButton(target webapi.ControlTarget) (controlButton, bool) {
	_, siteID := p.client.ContextIDs()
	mac, valid := normalizeClientMAC(target.MAC)
	if !valid || siteID == "" || target.SiteID != siteID {
		return controlButton{}, false
	}
	action := target.Action
	switch action {
	case webapi.ControlActionRebootDevice:
		if target.DeviceType != "ap" && target.DeviceType != "switch" && target.DeviceType != "gateway" {
			return controlButton{}, false
		}
	case webapi.ControlActionReconnectWiFiClient:
		action = controlReconnect
	default:
		return controlButton{}, false
	}
	siteKey := controlSiteKey(siteID, p.client.Config.Site)
	button := p.controlButtonTopics(siteKey, action, mac)
	button.SiteID = siteID
	button.Attributes = map[string]any{
		"omada_control": action, "target_mac": mac, "site": p.client.Config.Site, "site_id": siteID,
	}
	if p.client.Config.BridgeID != "" {
		button.Attributes["bridge_id"] = p.client.Config.BridgeID
	}
	labels := p.scopedLabels(map[string]string{"site": p.client.Config.Site, "site_id": siteID})
	metric, name, icon := "omada_device_status", "Reboot device", "mdi:restart"
	if action == controlReboot {
		labels["device_mac"], labels["device_name"], labels["device_model"] = mac, target.Name, target.Model
	} else {
		metric, name, icon = "omada_client_device_tracker", "Reconnect Wi-Fi client", "mdi:wifi-refresh"
		labels["mac"], labels["name"] = mac, target.Name
	}
	button.Config = map[string]any{
		"name": name, "unique_id": p.discoveryNode() + "_" + p.controlObjectID(siteKey, action, mac),
		"object_id": p.controlObjectID(siteKey, action, mac), "icon": icon,
		"command_topic": button.CommandTopic, "payload_press": "PRESS", "qos": 0, "retain": false,
		"json_attributes_topic": button.AttributesTopic, "entity_category": "config",
		"availability_mode": "all",
		"availability": []map[string]string{
			{"topic": p.availabilityTopic, "payload_available": "online", "payload_not_available": "offline"},
			{"topic": p.controlAvailabilityTopic(), "payload_available": "online", "payload_not_available": "offline"},
			{"topic": button.AvailabilityTopic, "payload_available": "online", "payload_not_available": "offline"},
		},
		"device": deviceInfo(p.client, metric, labels),
		"origin": map[string]string{"name": "omada_exporter", "support_url": "https://github.com/RCooLeR/omada_exporter"},
	}
	return button, true
}

func (p *Publisher) controlObjectID(siteKey, action, mac string) string {
	return "omada_control_" + siteKey + "_" + action + "_" + trackerID(mac)
}

func (p *Publisher) controlButtonTopics(siteKey, action, mac string) controlButton {
	base := fmt.Sprintf("%s/controls/%s/%s/%s", p.statePrefix(), siteKey, action, trackerID(mac))
	return controlButton{
		Action: action, MAC: mac, CommandTopic: base + "/command", AttributesTopic: base + "/attributes", AvailabilityTopic: base + "/status",
		DiscoveryTopic: fmt.Sprintf("%s/button/%s/%s/config", topicPrefix(p.client.Config.MQTTDiscoveryPrefix), p.discoveryNode(), p.controlObjectID(siteKey, action, mac)),
	}
}

// Retained discovery is inventory only, never a source of eligible commands.
func (p *Publisher) handleRetainedControl(_ mqtt.Client, message mqtt.Message) {
	if p.controls == nil || !message.Retained() || len(message.Payload()) == 0 || len(message.Payload()) > 16384 {
		return
	}
	var config struct {
		CommandTopic    string `json:"command_topic"`
		AttributesTopic string `json:"json_attributes_topic"`
		Origin          struct {
			Name string `json:"name"`
		} `json:"origin"`
	}
	if json.Unmarshal(message.Payload(), &config) != nil || config.Origin.Name != "omada_exporter" {
		return
	}
	prefix := p.statePrefix() + "/controls/"
	if !strings.HasPrefix(config.CommandTopic, prefix) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(config.CommandTopic, prefix), "/")
	if len(parts) != 4 || len(parts[0]) != 10 || (parts[1] != controlReboot && parts[1] != controlReconnect) || parts[3] != "command" {
		return
	}
	for _, ch := range parts[0] {
		if !(ch >= '0' && ch <= '9') && !(ch >= 'a' && ch <= 'f') {
			return
		}
	}
	mac, ok := normalizeClientMAC(parts[2])
	if !ok {
		return
	}
	button := p.controlButtonTopics(parts[0], parts[1], mac)
	if button.CommandTopic != config.CommandTopic || button.AttributesTopic != config.AttributesTopic || button.DiscoveryTopic != message.Topic() {
		return
	}
	c := p.controls
	c.mu.Lock()
	// Bound retained inventory independently of the number of incoming packets.
	if len(c.retained) < 10000 {
		c.retained[button.DiscoveryTopic] = button
	}
	c.mu.Unlock()
	notifyControl(c.reconcile)
}

func (p *Publisher) handleControlMessage(_ mqtt.Client, message mqtt.Message) {
	if p.controls == nil {
		return
	}
	c := p.controls
	c.mu.Lock()
	defer c.mu.Unlock()
	button, known := c.buttons[message.Topic()]
	if !known {
		return
	} // Never acknowledge or adopt arbitrary target topics.
	now := time.Now().UTC()
	eligible := p.client.Config.AllowDeviceControl && c.ctx != nil && c.ctx.Err() == nil && c.connected && c.ready && now.Sub(c.validatedAt) <= controlInventoryMaxAge && p.currentControlButton(button)
	if !eligible || c.pending || message.Retained() || message.Duplicate() || message.Qos() != 0 || len(message.Payload()) != 5 || string(message.Payload()) != "PRESS" || now.Sub(c.lastAttempt[button.CommandTopic]) < controlCooldown {
		p.setControlStatusLocked(button.CommandTopic, "rejected", now)
		return
	}
	c.pending = true
	c.lastAttempt[button.CommandTopic] = now
	// pending permits at most one queued or executing mutation across all targets.
	select {
	case c.commands <- controlCommand{Topic: button.CommandTopic, Epoch: c.epoch, ReceivedAt: now}:
	default:
		c.pending = false
		p.setControlStatusLocked(button.CommandTopic, "rejected", now)
	}
}

func (p *Publisher) setControlStatusLocked(topic, status string, now time.Time) {
	c := p.controls
	button, ok := c.buttons[topic]
	if !ok {
		return
	}
	button.Attributes["last_status"], button.Attributes["last_status_at"] = status, now.Format(time.RFC3339)
	c.buttons[topic] = button
	c.statusDirty[topic] = true
	notifyControl(c.status)
}

func (p *Publisher) runControls(ctx context.Context) {
	ticker := time.NewTicker(controlRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.controls.refresh:
			p.refreshControls(ctx)
		case <-ticker.C:
			p.refreshControls(ctx)
		case <-p.controls.reconcile:
			p.reconcileControls()
		case <-p.controls.status:
			p.publishControlStatuses()
		case topic := <-p.controls.commands:
			p.executeControl(ctx, topic)
		}
	}
}

func (p *Publisher) refreshControls(ctx context.Context) {
	c := p.controls
	c.mu.Lock()
	connected, epoch := c.connected, c.epoch
	c.mu.Unlock()
	if !connected || ctx.Err() != nil {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	targets, err := c.api.ControlTargets(requestCtx)
	cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != epoch || ctx.Err() != nil {
		return
	}
	if err != nil || !p.client.Config.AllowDeviceControl {
		c.ready = false
		p.publishBytes(p.controlAvailabilityTopic(), []byte("offline"), true)
		for _, button := range c.buttons {
			p.publishBytes(button.AvailabilityTopic, []byte("offline"), true)
		}
		return
	}
	current := map[string]controlButton{}
	ambiguous := map[string]bool{}
	for _, target := range targets {
		button, ok := p.makeControlButton(target)
		if !ok {
			continue
		}
		if ambiguous[button.CommandTopic] {
			continue
		}
		if _, duplicate := current[button.CommandTopic]; duplicate {
			delete(current, button.CommandTopic)
			ambiguous[button.CommandTopic] = true
			continue
		}
		if previous, ok := c.buttons[button.CommandTopic]; ok {
			for _, key := range []string{"last_status", "last_status_at"} {
				if value, present := previous.Attributes[key]; present {
					button.Attributes[key] = value
				}
			}
		}
		current[button.CommandTopic] = button
	}
	for _, button := range c.buttons {
		c.retained[button.DiscoveryTopic] = button
	}
	c.buttons = current
	c.validatedAt = time.Now().UTC()
	for topic, attemptedAt := range c.lastAttempt {
		if time.Since(attemptedAt) >= controlCooldown {
			delete(c.lastAttempt, topic)
		}
	}
	allPublished := true
	for _, button := range current {
		allPublished = p.publishControlJSON(button.DiscoveryTopic, button.Config) && allPublished
		allPublished = p.publishControlJSON(button.AttributesTopic, button.Attributes) && allPublished
		allPublished = p.publishBytes(button.AvailabilityTopic, []byte("online"), true) && allPublished
	}
	c.ready = allPublished
	p.reconcileControlsLocked()
	status := "offline"
	if allPublished {
		status = "online"
	}
	if !p.publishBytes(p.controlAvailabilityTopic(), []byte(status), true) {
		c.ready = false
	}
}

func (p *Publisher) reconcileControls() {
	c := p.controls
	c.mu.Lock()
	defer c.mu.Unlock()
	p.reconcileControlsLocked()
}

func (p *Publisher) reconcileControlsLocked() {
	c := p.controls
	if !c.ready || !c.connected {
		return
	}
	for topic, old := range c.retained {
		if _, exists := c.buttons[old.CommandTopic]; exists {
			delete(c.retained, topic)
			continue
		}
		p.publishBytes(old.AvailabilityTopic, []byte("offline"), true)
		if p.publishBytes(topic, nil, true) && p.publishBytes(old.AttributesTopic, nil, true) {
			delete(c.retained, topic)
			delete(c.statusDirty, old.CommandTopic)
		}
	}
}

func (p *Publisher) publishControlStatuses() {
	c := p.controls
	c.mu.Lock()
	defer c.mu.Unlock()
	for topic := range c.statusDirty {
		if button, ok := c.buttons[topic]; ok {
			p.publishJSON(button.AttributesTopic, maps.Clone(button.Attributes), true)
		}
		delete(c.statusDirty, topic)
	}
}

func (p *Publisher) publishControlJSON(topic string, payload any) bool {
	body, err := json.Marshal(payload)
	return err == nil && p.publishBytes(topic, body, true)
}

func (p *Publisher) currentControlButton(button controlButton) bool {
	_, siteID := p.client.ContextIDs()
	return siteID != "" && siteID == button.SiteID && button.CommandTopic == p.controlButtonTopics(controlSiteKey(siteID, p.client.Config.Site), button.Action, button.MAC).CommandTopic
}

func (p *Publisher) executeControl(ctx context.Context, command controlCommand) {
	c := p.controls
	topic := command.Topic
	c.mu.Lock()
	button, known := c.buttons[topic]
	eligible := known && p.client.Config.AllowDeviceControl && c.connected && c.ready && ctx.Err() == nil && time.Since(c.validatedAt) <= controlInventoryMaxAge && c.epoch == command.Epoch && time.Since(command.ReceivedAt) <= controlCommandMaxAge && p.currentControlButton(button)
	if !eligible {
		c.pending = false
		p.setControlStatusLocked(topic, "rejected", time.Now().UTC())
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	var err error
	if button.Action == controlReboot {
		err = c.api.RebootDevice(requestCtx, button.SiteID, button.MAC)
	} else {
		err = c.api.ReconnectWiFiClient(requestCtx, button.SiteID, button.MAC)
	}
	cancel()
	c.mu.Lock()
	c.pending = false
	status := "accepted"
	if err != nil {
		status = "failed"
		c.ready = false
	}
	p.setControlStatusLocked(topic, status, time.Now().UTC())
	c.mu.Unlock()
	if err != nil {
		p.publishBytes(p.controlAvailabilityTopic(), []byte("offline"), true)
	}
	// Do not retry a mutation, even on timeout. Refresh only read-only eligibility.
	notifyControl(c.refresh)
}
