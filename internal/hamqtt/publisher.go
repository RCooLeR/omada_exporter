package hamqtt

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"
)

type Publisher struct {
	client            *api.Client
	registry          *prometheus.Registry
	mqtt              mqtt.Client
	availabilityTopic string
	published         map[string]struct{}
	knownClients      map[string]clientTracker
	mu                sync.Mutex
}

type clientTracker struct {
	StateTopic      string
	AttributesTopic string
	Labels          map[string]string
}

type entity struct {
	Component      string
	ObjectID       string
	UniqueID       string
	Name           string
	DiscoveryTopic string
	StateTopic     string
	MetricName     string
	Help           string
	Labels         map[string]string
	Device         map[string]any
}

var slugPattern = regexp.MustCompile(`[^a-z0-9_]+`)

func NewPublisher(client *api.Client, collectors map[string]prometheus.Collector) (*Publisher, error) {
	registry := prometheus.NewRegistry()
	for name, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register mqtt collector %s: %w", name, err)
		}
	}

	prefix := topicPrefix(client.Config.MQTTTopicPrefix)
	return &Publisher{
		client:            client,
		registry:          registry,
		availabilityTopic: prefix + "/status",
		published:         map[string]struct{}{},
		knownClients:      map[string]clientTracker{},
	}, nil
}

func (p *Publisher) Run(ctx context.Context) error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(normalizeBroker(p.client.Config.MQTTBroker))
	opts.SetClientID(p.client.Config.MQTTClientID)
	opts.SetUsername(p.client.Config.MQTTUsername)
	opts.SetPassword(p.client.Config.MQTTPassword)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetCleanSession(true)
	opts.SetWill(p.availabilityTopic, "offline", 0, true)
	opts.OnConnect = func(client mqtt.Client) {
		log.Info().Msg("connected to mqtt broker")
		p.publishBytes(p.availabilityTopic, []byte("online"), true)
	}
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Warn().Err(err).Msg("mqtt connection lost")
	}

	p.mqtt = mqtt.NewClient(opts)
	if token := p.mqtt.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	p.publishBytes(p.availabilityTopic, []byte("online"), true)
	p.publishAll()

	interval := time.Duration(p.client.Config.MQTTInterval) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.publishBytes(p.availabilityTopic, []byte("offline"), true)
			p.mqtt.Disconnect(250)
			return ctx.Err()
		case <-ticker.C:
			p.publishAll()
		}
	}
}

func (p *Publisher) publishAll() {
	families, err := p.registry.Gather()
	if err != nil {
		log.Error().Err(err).Msg("failed to gather mqtt metrics")
		return
	}

	seenClients := map[string]clientTracker{}
	for _, family := range families {
		for _, metric := range family.Metric {
			value, ok := metricValue(metric)
			if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}

			labels := metricLabels(metric)
			ent := p.newMetricEntity(family, labels)
			p.publishDiscovery(ent, family.GetType())
			p.publishMetricState(ent, value)

			if tracker, ok := p.clientTracker(family.GetName(), labels); ok {
				seenClients[trackerID(labels["mac"])] = tracker
			}
		}
	}

	p.publishClientTrackers(seenClients)
}

func (p *Publisher) publishDiscovery(ent entity, metricType dto.MetricType) {
	p.mu.Lock()
	if _, ok := p.published[ent.DiscoveryTopic]; ok {
		if p.client.Config.MQTTRetain {
			p.mu.Unlock()
			return
		}
	}
	p.published[ent.DiscoveryTopic] = struct{}{}
	p.mu.Unlock()

	config := map[string]any{
		"name":                  ent.Name,
		"unique_id":             ent.UniqueID,
		"object_id":             ent.ObjectID,
		"state_topic":           ent.StateTopic,
		"value_template":        "{{ value_json.value }}",
		"json_attributes_topic": ent.StateTopic,
		"availability_topic":    p.availabilityTopic,
		"payload_available":     "online",
		"payload_not_available": "offline",
		"device":                ent.Device,
		"origin": map[string]any{
			"name":        "omada_exporter",
			"sw_version":  "omada_exporter",
			"support_url": "https://github.com/RCooLeR/omada_exporter",
		},
	}

	if ent.Component == "binary_sensor" {
		config["value_template"] = "{{ value_json.value | int }}"
		config["payload_on"] = "1"
		config["payload_off"] = "0"
		if deviceClass := binaryDeviceClass(ent.MetricName); deviceClass != "" {
			config["device_class"] = deviceClass
		}
	} else {
		for k, v := range sensorHints(ent.MetricName, metricType) {
			config[k] = v
		}
	}

	if p.client.Config.MQTTExpireAfter > 0 && ent.Component == "sensor" {
		config["expire_after"] = p.client.Config.MQTTExpireAfter
	}

	p.publishJSON(ent.DiscoveryTopic, config, p.client.Config.MQTTRetain)
}

func (p *Publisher) publishMetricState(ent entity, value float64) {
	payload := map[string]any{
		"value":        metricPayloadValue(value),
		"metric":       ent.MetricName,
		"help":         ent.Help,
		"last_updated": time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range ent.Labels {
		payload[k] = v
	}
	p.publishJSON(ent.StateTopic, payload, p.client.Config.MQTTRetain)
}

func (p *Publisher) newMetricEntity(family *dto.MetricFamily, labels map[string]string) entity {
	metricName := family.GetName()
	component := "sensor"
	if isBinaryMetric(metricName) {
		component = "binary_sensor"
	}

	objectID := objectID(metricName, labels)
	discoveryPrefix := topicPrefix(p.client.Config.MQTTDiscoveryPrefix)
	statePrefix := topicPrefix(p.client.Config.MQTTTopicPrefix)

	return entity{
		Component:      component,
		ObjectID:       objectID,
		UniqueID:       "omada_exporter_" + objectID,
		Name:           friendlyMetricName(metricName, labels),
		DiscoveryTopic: fmt.Sprintf("%s/%s/omada_exporter/%s/config", discoveryPrefix, component, objectID),
		StateTopic:     fmt.Sprintf("%s/entities/%s/state", statePrefix, objectID),
		MetricName:     metricName,
		Help:           family.GetHelp(),
		Labels:         labels,
		Device:         deviceInfo(p.client, metricName, labels),
	}
}

func (p *Publisher) clientTracker(metricName string, labels map[string]string) (clientTracker, bool) {
	if !strings.HasPrefix(metricName, "omada_client_") || labels["mac"] == "" {
		return clientTracker{}, false
	}

	id := trackerID(labels["mac"])
	statePrefix := topicPrefix(p.client.Config.MQTTTopicPrefix)
	return clientTracker{
		StateTopic:      fmt.Sprintf("%s/device_trackers/%s/state", statePrefix, id),
		AttributesTopic: fmt.Sprintf("%s/device_trackers/%s/attributes", statePrefix, id),
		Labels:          copyLabels(labels),
	}, true
}

func (p *Publisher) publishClientTrackers(seen map[string]clientTracker) {
	for id, tracker := range seen {
		p.publishClientTrackerDiscovery(id, tracker)

		attributes := map[string]any{
			"last_seen": time.Now().UTC().Format(time.RFC3339),
		}
		for k, v := range tracker.Labels {
			attributes[k] = v
		}
		p.publishBytes(tracker.StateTopic, []byte("home"), p.client.Config.MQTTRetain)
		p.publishJSON(tracker.AttributesTopic, attributes, p.client.Config.MQTTRetain)
	}

	p.mu.Lock()
	previous := p.knownClients
	p.knownClients = seen
	p.mu.Unlock()

	for id, tracker := range previous {
		if _, ok := seen[id]; ok {
			continue
		}
		p.publishBytes(tracker.StateTopic, []byte("not_home"), p.client.Config.MQTTRetain)
	}
}

func (p *Publisher) publishClientTrackerDiscovery(id string, tracker clientTracker) {
	discoveryTopic := fmt.Sprintf("%s/device_tracker/omada_exporter/%s/config", topicPrefix(p.client.Config.MQTTDiscoveryPrefix), id)

	p.mu.Lock()
	if _, ok := p.published[discoveryTopic]; ok {
		if p.client.Config.MQTTRetain {
			p.mu.Unlock()
			return
		}
	}
	p.published[discoveryTopic] = struct{}{}
	p.mu.Unlock()

	config := map[string]any{
		"name":                  clientName(tracker.Labels),
		"unique_id":             "omada_client_" + id,
		"object_id":             "omada_client_" + id,
		"state_topic":           tracker.StateTopic,
		"json_attributes_topic": tracker.AttributesTopic,
		"source_type":           "router",
		"payload_home":          "home",
		"payload_not_home":      "not_home",
		"availability_topic":    p.availabilityTopic,
		"payload_available":     "online",
		"payload_not_available": "offline",
		"device":                deviceInfo(p.client, "omada_client_device_tracker", tracker.Labels),
		"origin": map[string]any{
			"name":        "omada_exporter",
			"sw_version":  "omada_exporter",
			"support_url": "https://github.com/RCooLeR/omada_exporter",
		},
	}
	p.publishJSON(discoveryTopic, config, p.client.Config.MQTTRetain)
}

func (p *Publisher) publishJSON(topic string, payload any, retained bool) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Str("topic", topic).Msg("failed to encode mqtt payload")
		return
	}
	p.publishBytes(topic, body, retained)
}

func (p *Publisher) publishBytes(topic string, payload []byte, retained bool) {
	if p.mqtt == nil || !p.mqtt.IsConnected() {
		return
	}
	token := p.mqtt.Publish(topic, 0, retained, payload)
	if !token.WaitTimeout(10 * time.Second) {
		log.Warn().Str("topic", topic).Msg("mqtt publish timed out")
		return
	}
	if err := token.Error(); err != nil {
		log.Error().Err(err).Str("topic", topic).Msg("mqtt publish failed")
	}
}

func metricValue(metric *dto.Metric) (float64, bool) {
	if metric.Gauge != nil {
		return metric.Gauge.GetValue(), true
	}
	if metric.Counter != nil {
		return metric.Counter.GetValue(), true
	}
	if metric.Untyped != nil {
		return metric.Untyped.GetValue(), true
	}
	return 0, false
}

func metricPayloadValue(value float64) any {
	const (
		maxInt64AsFloat = float64(1<<63 - 1)
		minInt64AsFloat = -float64(1 << 63)
	)
	if value == math.Trunc(value) && value <= maxInt64AsFloat && value >= minInt64AsFloat {
		return int64(value)
	}
	return value
}

func metricLabels(metric *dto.Metric) map[string]string {
	labels := make(map[string]string, len(metric.Label))
	for _, label := range metric.Label {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}

func isBinaryMetric(name string) bool {
	switch name {
	case "omada_controller_upgrade_available",
		"omada_device_need_upgrade",
		"omada_port_link_status",
		"omada_lag_link_status",
		"omada_isp_status",
		"omada_vpn_status":
		return true
	default:
		return false
	}
}

func binaryDeviceClass(name string) string {
	switch name {
	case "omada_controller_upgrade_available", "omada_device_need_upgrade":
		return "problem"
	case "omada_port_link_status", "omada_lag_link_status", "omada_isp_status", "omada_vpn_status":
		return "connectivity"
	default:
		return ""
	}
}

func sensorHints(name string, metricType dto.MetricType) map[string]any {
	hints := map[string]any{}
	lower := strings.ToLower(name)

	if metricType == dto.MetricType_COUNTER {
		hints["state_class"] = "total_increasing"
	} else {
		hints["state_class"] = "measurement"
	}

	switch {
	case strings.HasSuffix(lower, "_bytes"):
		hints["unit_of_measurement"] = "B"
		hints["device_class"] = "data_size"
	case strings.HasSuffix(lower, "_seconds") || strings.HasSuffix(lower, "_uptime"):
		hints["unit_of_measurement"] = "s"
		hints["device_class"] = "duration"
	case strings.Contains(lower, "latency"):
		hints["unit_of_measurement"] = "ms"
		hints["device_class"] = "duration"
	case strings.Contains(lower, "percentage") || strings.HasSuffix(lower, "_pct") || strings.HasSuffix(lower, "_util"):
		hints["unit_of_measurement"] = "%"
	case strings.HasSuffix(lower, "_watts"):
		hints["unit_of_measurement"] = "W"
		hints["device_class"] = "power"
	case strings.Contains(lower, "_temp"):
		hints["unit_of_measurement"] = "°C"
		hints["device_class"] = "temperature"
	case strings.HasSuffix(lower, "_mbps"):
		hints["unit_of_measurement"] = "Mbit/s"
	case strings.Contains(lower, "_rate") || strings.Contains(lower, "_speed"):
		hints["unit_of_measurement"] = "bit/s"
	case strings.Contains(lower, "_download") || strings.Contains(lower, "_upload"):
		hints["unit_of_measurement"] = "B"
	}

	return hints
}

func friendlyMetricName(metricName string, labels map[string]string) string {
	base := strings.TrimPrefix(metricName, "omada_")
	parts := strings.Split(base, "_")
	for i, part := range parts {
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	name := strings.Join(parts, " ")

	qualifiers := []string{}
	for _, key := range []string{"storage_name", "upgrade_channel", "port", "lag_id", "name", "connection_mode", "wifi_mode", "ssid"} {
		value := strings.TrimSpace(labels[key])
		if value == "" {
			continue
		}
		switch key {
		case "port":
			qualifiers = append(qualifiers, "Port "+value)
		case "lag_id":
			qualifiers = append(qualifiers, "LAG "+value)
		default:
			qualifiers = append(qualifiers, value)
		}
	}
	if len(qualifiers) > 0 {
		name += " " + strings.Join(qualifiers, " ")
	}
	return name
}

func objectID(metricName string, labels map[string]string) string {
	stable := []string{metricName}

	for _, key := range []string{"site_id", "site", "device_mac", "mac", "gateway_mac", "vpn_id", "storage_name", "upgrade_channel", "port", "lag_id"} {
		if value := labels[key]; value != "" {
			stable = append(stable, key+"_"+value)
		}
	}

	if labels["device_mac"] == "" && labels["mac"] == "" && labels["gateway_mac"] == "" && labels["vpn_id"] == "" {
		for _, key := range []string{"interface_name", "local_ip", "remote_ip", "connection_mode", "wifi_mode", "ssid", "name"} {
			if value := labels[key]; value != "" {
				stable = append(stable, key+"_"+value)
			}
		}
	}

	return slug(strings.Join(stable, "_")) + "_" + shortHash(stable)
}

func shortHash(values []string) string {
	parts := append([]string{}, values...)
	sort.Strings(parts)
	h := sha1.New()
	for _, value := range parts {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

func slug(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, ".", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = slugPattern.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	if value == "" {
		return "omada"
	}
	if len(value) > 180 {
		value = value[:180]
		value = strings.Trim(value, "_")
	}
	return value
}

func topicPrefix(prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "omada_exporter"
	}
	return prefix
}

func normalizeBroker(broker string) string {
	if strings.Contains(broker, "://") {
		return broker
	}
	return "tcp://" + broker
}

func deviceInfo(client *api.Client, metricName string, labels map[string]string) map[string]any {
	if strings.HasPrefix(metricName, "omada_client_") && labels["mac"] != "" {
		device := map[string]any{
			"identifiers":  []string{"omada_client_" + trackerID(labels["mac"])},
			"name":         clientName(labels),
			"manufacturer": firstNonEmpty(labels["vendor"], "Unknown"),
			"model":        firstNonEmpty(labels["device_type"], labels["device_category"]),
		}
		return compactDevice(device)
	}

	if labels["device_mac"] != "" {
		device := map[string]any{
			"identifiers":       []string{"omada_device_" + trackerID(labels["device_mac"])},
			"name":              firstNonEmpty(labels["device_name"], labels["device_mac"]),
			"manufacturer":      "TP-Link",
			"model":             firstNonEmpty(labels["device_show_model"], labels["device_model"]),
			"sw_version":        labels["device_version"],
			"hw_version":        labels["device_hw_version"],
			"configuration_url": client.Config.Host,
		}
		return compactDevice(device)
	}

	if labels["gateway_mac"] != "" {
		device := map[string]any{
			"identifiers":       []string{"omada_device_" + trackerID(labels["gateway_mac"])},
			"name":              firstNonEmpty(labels["gateway_name"], labels["gateway_mac"]),
			"manufacturer":      "TP-Link",
			"configuration_url": client.Config.Host,
		}
		return compactDevice(device)
	}

	if labels["vpn_id"] != "" {
		device := map[string]any{
			"identifiers":  []string{"omada_vpn_" + slug(labels["vpn_id"])},
			"name":         firstNonEmpty(labels["name"], labels["vpn_id"]),
			"manufacturer": "TP-Link Omada",
			"model":        firstNonEmpty(labels["vpn_type"], "VPN"),
		}
		return compactDevice(device)
	}

	if strings.HasPrefix(metricName, "omada_vpn_") && labels["name"] != "" {
		device := map[string]any{
			"identifiers":  []string{"omada_vpn_" + slug(labels["name"]+"_"+labels["interface_name"]+"_"+labels["remote_ip"])},
			"name":         labels["name"],
			"manufacturer": "TP-Link Omada",
			"model":        firstNonEmpty(labels["vpn_type"], "VPN"),
		}
		return compactDevice(device)
	}

	siteID := firstNonEmpty(labels["site_id"], client.SiteId, labels["site"], client.Config.Site)
	siteName := firstNonEmpty(labels["site"], client.Config.Site, "Omada Site")
	return compactDevice(map[string]any{
		"identifiers":       []string{"omada_site_" + slug(siteID)},
		"name":              "Omada " + siteName,
		"manufacturer":      "TP-Link Omada",
		"model":             "Site",
		"configuration_url": client.Config.Host,
	})
}

func compactDevice(device map[string]any) map[string]any {
	for key, value := range device {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				delete(device, key)
			}
		case []string:
			if len(typed) == 0 || strings.TrimSpace(typed[0]) == "" {
				delete(device, key)
			}
		}
	}
	return device
}

func clientName(labels map[string]string) string {
	return firstNonEmpty(labels["name"], labels["host_name"], labels["system_name"], labels["ip"], labels["mac"], "Omada Client")
}

func trackerID(mac string) string {
	return slug(strings.ReplaceAll(strings.ToLower(mac), ":", "_"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func copyLabels(labels map[string]string) map[string]string {
	copied := make(map[string]string, len(labels))
	for key, value := range labels {
		copied[key] = value
	}
	return copied
}
