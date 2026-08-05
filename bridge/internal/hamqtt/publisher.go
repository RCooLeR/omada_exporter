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
	"unicode"

	"github.com/RCooLeR/omada_exporter/internal/api"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"
)

// Publisher publishes collected metrics to Home Assistant over MQTT.
type Publisher struct {
	client             *api.Client
	registry           *prometheus.Registry
	mqtt               mqtt.Client
	availabilityTopic  string
	published          map[string]struct{}
	knownClients       map[string]clientTracker
	lastClientTrackers map[string]clientTracker
	clientLastSeen     map[string]time.Time
	clientAttributes   map[string]string
	trackedClientMACs  []string
	metricSamples      map[string]metricSample
	retainedDiscovery  map[string]string
	retainedStates     map[string]map[string]string
	retainedLoaded     bool
	mu                 sync.Mutex
}

// clientTracker stores MQTT topics and labels for a tracked client.
type clientTracker struct {
	StateTopic      string
	AttributesTopic string
	Labels          map[string]string
}

type retainedClientAttributes struct {
	Topic    string
	Payload  string
	Labels   map[string]string
	LastSeen time.Time
}

// entity describes a Home Assistant entity generated from a metric.
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

// metricSample stores the last observed metric value and timestamp.
type metricSample struct {
	Value      float64
	ObservedAt time.Time
}

// publishContext stores lookup data used while publishing MQTT entities.
type publishContext struct {
	vpnIDByModeTypeName map[string]string
	vpnIDByName         map[string]string
	infrastructureByMAC map[string]map[string]string
}

var slugPattern = regexp.MustCompile(`[^a-z0-9_]+`)

const (
	mqttConnectTimeout       = 10 * time.Second
	mqttConnectRetryInterval = 30 * time.Second
)

// NewPublisher creates an MQTT publisher for the configured collectors.
func NewPublisher(client *api.Client, collectors map[string]prometheus.Collector) (*Publisher, error) {
	registry := prometheus.NewRegistry()
	for name, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register mqtt collector %s: %w", name, err)
		}
	}

	prefix := topicPrefix(client.Config.MQTTTopicPrefix)
	return &Publisher{
		client:             client,
		registry:           registry,
		availabilityTopic:  prefix + "/status",
		published:          map[string]struct{}{},
		knownClients:       map[string]clientTracker{},
		lastClientTrackers: map[string]clientTracker{},
		clientLastSeen:     map[string]time.Time{},
		clientAttributes:   map[string]string{},
		trackedClientMACs:  parseTrackedClientMACs(client.Config.MQTTTrackedClientMACs),
		metricSamples:      map[string]metricSample{},
		retainedDiscovery:  map[string]string{},
		retainedStates:     map[string]map[string]string{},
	}, nil
}

// Run connects to MQTT and publishes metric updates on a schedule.
func (p *Publisher) Run(ctx context.Context) error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(normalizeBroker(p.client.Config.MQTTBroker))
	opts.SetClientID(p.client.Config.MQTTClientID)
	opts.SetUsername(p.client.Config.MQTTUsername)
	opts.SetPassword(p.client.Config.MQTTPassword)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(mqttConnectRetryInterval)
	opts.SetConnectRetry(false)
	opts.SetConnectTimeout(mqttConnectTimeout)
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
	if err := p.connect(ctx); err != nil {
		return err
	}

	p.publishBytes(p.availabilityTopic, []byte("online"), true)
	if p.client.Config.MQTTRetain {
		if err := p.loadRetainedInventory(ctx); err != nil {
			log.Warn().Err(err).Msg("retained mqtt inventory unavailable; automatic superseded-entity cleanup disabled")
		}
	}
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

func (p *Publisher) connect(ctx context.Context) error {
	for {
		token := p.mqtt.Connect()
		token.Wait()
		if err := token.Error(); err == nil {
			return nil
		} else {
			log.Warn().Err(err).Dur("retry_after", mqttConnectRetryInterval).Msg("mqtt broker unavailable")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(mqttConnectRetryInterval):
		}
	}
}

// publishAll gathers metrics and publishes discovery and state updates.
func (p *Publisher) publishAll() {
	families, err := p.registry.Gather()
	if err != nil {
		log.Error().Err(err).Msg("failed to gather mqtt metrics")
		return
	}

	now := time.Now().UTC()
	ctx := buildPublishContext(families)
	seenClients := map[string]clientTracker{}
	currentEntities := map[string]entity{}
	for _, family := range families {
		for _, metric := range family.Metric {
			value, ok := metricValue(metric)
			if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}

			labels := metricLabels(metric)
			ent := p.newMetricEntity(family, labels, ctx)
			currentEntities[ent.DiscoveryTopic] = ent
			p.publishDiscovery(ent, family.GetType())
			p.publishMetricState(ent, value, now)
			if derived, ok := p.publishDerivedMetricState(family.GetName(), labels, value, now, ctx); ok {
				currentEntities[derived.DiscoveryTopic] = derived
			}

			if tracker, ok := p.clientTracker(family.GetName(), labels); ok {
				id := trackerID(labels["mac"])
				_, infrastructure := ctx.infrastructureByMAC[id]
				if !infrastructure || p.isExplicitlyTrackedClient(id) {
					seenClients[id] = tracker
				}
			}
		}
	}

	p.publishClientTrackers(seenClients)
	p.removeInfrastructureClientTrackers(ctx)
	p.reconcileSupersededEntities(currentEntities)
}

// publishDiscovery publishes Home Assistant discovery data for a metric entity.
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

// publishMetricState publishes the current state payload for an entity.
func (p *Publisher) publishMetricState(ent entity, value float64, observedAt time.Time) {
	payload := map[string]any{
		"value":        metricPayloadValue(value),
		"metric":       ent.MetricName,
		"help":         ent.Help,
		"last_updated": observedAt.Format(time.RFC3339),
	}
	for k, v := range ent.Labels {
		payload[k] = v
	}
	p.publishJSON(ent.StateTopic, payload, p.client.Config.MQTTRetain)
}

// publishDerivedMetricState publishes derived values calculated from a metric.
func (p *Publisher) publishDerivedMetricState(metricName string, labels map[string]string, value float64, observedAt time.Time, ctx publishContext) (entity, bool) {
	derivedMetricName, help, ok := derivedMetric(metricName)
	if !ok {
		return entity{}, false
	}

	sampleKey := objectID(metricName, labels)
	rate := p.recordRateSample(sampleKey, value, observedAt)

	ent := p.newDerivedEntity(derivedMetricName, help, labels, ctx)
	p.publishDiscovery(ent, dto.MetricType_GAUGE)
	p.publishMetricState(ent, rate, observedAt)
	return ent, true
}

// newMetricEntity builds a Home Assistant entity description from a metric family.
func (p *Publisher) newMetricEntity(family *dto.MetricFamily, labels map[string]string, ctx publishContext) entity {
	metricName := family.GetName()
	component := "sensor"
	if isBinaryMetric(metricName) {
		component = "binary_sensor"
	}

	objectID := objectID(metricName, labels)
	discoveryPrefix := topicPrefix(p.client.Config.MQTTDiscoveryPrefix)
	statePrefix := topicPrefix(p.client.Config.MQTTTopicPrefix)
	discoveryTopic := fmt.Sprintf("%s/%s/omada_exporter/%s/config", discoveryPrefix, component, objectID)
	stateTopic := fmt.Sprintf("%s/entities/%s/state", statePrefix, objectID)
	objectID, discoveryTopic, stateTopic = p.canonicalEntityTopics(metricName, labels, component, objectID, discoveryTopic, stateTopic)

	return entity{
		Component:      component,
		ObjectID:       objectID,
		UniqueID:       "omada_exporter_" + objectID,
		Name:           friendlyMetricName(metricName, labels),
		DiscoveryTopic: discoveryTopic,
		StateTopic:     stateTopic,
		MetricName:     metricName,
		Help:           family.GetHelp(),
		Labels:         labels,
		Device:         deviceInfo(p.client, metricName, deviceLabels(metricName, labels, ctx)),
	}
}

// newDerivedEntity builds a Home Assistant entity description for a derived metric.
func (p *Publisher) newDerivedEntity(metricName, help string, labels map[string]string, ctx publishContext) entity {
	objectID := objectID(metricName, labels)
	discoveryPrefix := topicPrefix(p.client.Config.MQTTDiscoveryPrefix)
	statePrefix := topicPrefix(p.client.Config.MQTTTopicPrefix)
	discoveryTopic := fmt.Sprintf("%s/sensor/omada_exporter/%s/config", discoveryPrefix, objectID)
	stateTopic := fmt.Sprintf("%s/entities/%s/state", statePrefix, objectID)
	objectID, discoveryTopic, stateTopic = p.canonicalEntityTopics(metricName, labels, "sensor", objectID, discoveryTopic, stateTopic)

	return entity{
		Component:      "sensor",
		ObjectID:       objectID,
		UniqueID:       "omada_exporter_" + objectID,
		Name:           friendlyMetricName(metricName, labels),
		DiscoveryTopic: discoveryTopic,
		StateTopic:     stateTopic,
		MetricName:     metricName,
		Help:           help,
		Labels:         labels,
		Device:         deviceInfo(p.client, metricName, deviceLabels(metricName, labels, ctx)),
	}
}

// clientTracker returns tracker metadata for metrics that represent clients.
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

// publishClientTrackers publishes discovery and state updates for client trackers.
func (p *Publisher) publishClientTrackers(seen map[string]clientTracker) {
	observedAt := time.Now().UTC()

	p.mu.Lock()
	if p.lastClientTrackers == nil {
		p.lastClientTrackers = map[string]clientTracker{}
	}
	if p.clientLastSeen == nil {
		p.clientLastSeen = map[string]time.Time{}
	}
	previous := copyClientTrackers(p.knownClients)
	for id, tracker := range seen {
		p.lastClientTrackers[id] = tracker
		p.clientLastSeen[id] = observedAt
	}
	lastKnown := copyClientTrackers(p.lastClientTrackers)
	lastSeen := copyClientLastSeen(p.clientLastSeen)
	p.knownClients = seen
	p.mu.Unlock()

	// Clients that appear in this gather are currently online. We publish both
	// discovery and state so Home Assistant can create/update the tracker and
	// immediately mark it as home.
	for id, tracker := range seen {
		p.publishClientTrackerDiscovery(id, tracker)

		p.publishBytes(tracker.StateTopic, []byte("home"), p.client.Config.MQTTRetain)
		p.publishClientTrackerAttributes(tracker, false, time.Time{})
	}

	// Configured MAC addresses are special: the user wants Home Assistant to
	// know about them even if Omada did not report them in the current online
	// client list. If we have old labels from a previous online sighting, reuse
	// them so the tracker keeps its friendly name/vendor details while offline.
	configured := p.configuredClientTrackers()
	for id, tracker := range configured {
		if _, ok := seen[id]; ok {
			continue
		}
		if lastKnownTracker, ok := lastKnown[id]; ok {
			tracker = lastKnownTracker
		}

		p.publishClientTrackerDiscovery(id, tracker)

		p.publishBytes(tracker.StateTopic, []byte("not_home"), p.client.Config.MQTTRetain)
		p.publishClientTrackerAttributes(tracker, true, lastSeen[id])
	}

	// Dynamic clients are marked away only after this publisher has seen them
	// once. That avoids creating unlimited offline trackers for every historical
	// client unless the MAC was explicitly configured above.
	for id, tracker := range previous {
		if _, ok := seen[id]; ok {
			continue
		}
		if _, ok := configured[id]; ok {
			continue
		}
		p.publishBytes(tracker.StateTopic, []byte("not_home"), p.client.Config.MQTTRetain)
		p.publishClientTrackerAttributes(tracker, false, lastSeen[id])
	}
}

// publishClientTrackerAttributes publishes stable tracker metadata only when
// the effective Home Assistant attributes change. lastSeen is supplied only
// for an offline transition, so active clients do not generate a new Recorder
// write on every collection interval.
func (p *Publisher) publishClientTrackerAttributes(tracker clientTracker, configured bool, lastSeen time.Time) {
	attributes := make(map[string]any, len(tracker.Labels)+2)
	for key, value := range tracker.Labels {
		attributes[key] = value
	}
	if configured {
		attributes["configured"] = true
	}
	if !lastSeen.IsZero() {
		attributes["last_seen"] = lastSeen.Format(time.RFC3339)
	}

	body, err := json.Marshal(attributes)
	if err != nil {
		log.Error().Err(err).Str("topic", tracker.AttributesTopic).Msg("failed to encode mqtt payload")
		return
	}

	p.mu.Lock()
	if p.clientAttributes == nil {
		p.clientAttributes = map[string]string{}
	}
	if p.client.Config.MQTTRetain && p.clientAttributes[tracker.AttributesTopic] == string(body) {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	if !p.publishBytes(tracker.AttributesTopic, body, p.client.Config.MQTTRetain) {
		return
	}

	p.mu.Lock()
	p.clientAttributes[tracker.AttributesTopic] = string(body)
	p.mu.Unlock()
}

func copyClientLastSeen(source map[string]time.Time) map[string]time.Time {
	copy := make(map[string]time.Time, len(source))
	for id, observedAt := range source {
		copy[id] = observedAt
	}
	return copy
}

// removeInfrastructureClientTrackers prevents controllers, gateways, switches,
// and APs that also appear in Omada's active-client feed from becoming a
// second Home Assistant client device. Explicitly configured tracker MACs are
// preserved because that is a direct user request.
func (p *Publisher) removeInfrastructureClientTrackers(ctx publishContext) {
	discoveryPrefix := topicPrefix(p.client.Config.MQTTDiscoveryPrefix)
	statePrefix := topicPrefix(p.client.Config.MQTTTopicPrefix)
	for id := range ctx.infrastructureByMAC {
		if p.isExplicitlyTrackedClient(id) {
			continue
		}
		p.publishBytes(fmt.Sprintf("%s/device_tracker/omada_exporter/%s/config", discoveryPrefix, id), []byte{}, true)
		p.publishBytes(fmt.Sprintf("%s/device_trackers/%s/state", statePrefix, id), []byte{}, true)
		p.publishBytes(fmt.Sprintf("%s/device_trackers/%s/attributes", statePrefix, id), []byte{}, true)

		p.mu.Lock()
		delete(p.knownClients, id)
		delete(p.lastClientTrackers, id)
		delete(p.clientLastSeen, id)
		delete(p.clientAttributes, fmt.Sprintf("%s/device_trackers/%s/attributes", statePrefix, id))
		p.mu.Unlock()
	}
}

func (p *Publisher) isExplicitlyTrackedClient(id string) bool {
	for _, mac := range p.trackedClientMACs {
		if trackerID(mac) == id {
			return true
		}
	}
	return false
}

// configuredClientTrackers builds device trackers for explicitly configured MAC addresses.
func (p *Publisher) configuredClientTrackers() map[string]clientTracker {
	trackers := make(map[string]clientTracker, len(p.trackedClientMACs))
	statePrefix := topicPrefix(p.client.Config.MQTTTopicPrefix)
	for _, mac := range p.trackedClientMACs {
		id := trackerID(mac)
		labels := map[string]string{
			"mac": mac,
		}
		if p.client.Config.Site != "" {
			labels["site"] = p.client.Config.Site
		}
		if p.client.SiteId != "" {
			labels["site_id"] = p.client.SiteId
		}

		trackers[id] = clientTracker{
			StateTopic:      fmt.Sprintf("%s/device_trackers/%s/state", statePrefix, id),
			AttributesTopic: fmt.Sprintf("%s/device_trackers/%s/attributes", statePrefix, id),
			Labels:          labels,
		}
	}
	return trackers
}

// publishClientTrackerDiscovery publishes Home Assistant discovery data for a client tracker.
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

// publishJSON marshals a payload and publishes it as JSON.
func (p *Publisher) publishJSON(topic string, payload any, retained bool) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Str("topic", topic).Msg("failed to encode mqtt payload")
		return
	}
	p.publishBytes(topic, body, retained)
}

// publishBytes publishes a raw MQTT payload and reports whether the broker
// accepted it.
func (p *Publisher) publishBytes(topic string, payload []byte, retained bool) bool {
	if p.mqtt == nil || !p.mqtt.IsConnectionOpen() {
		return false
	}
	token := p.mqtt.Publish(topic, 0, retained, payload)
	if !token.WaitTimeout(10 * time.Second) {
		log.Warn().Str("topic", topic).Msg("mqtt publish timed out")
		return false
	}
	if err := token.Error(); err != nil {
		log.Error().Err(err).Str("topic", topic).Msg("mqtt publish failed")
		return false
	}
	return true
}

// loadRetainedInventory takes a bounded snapshot of metric discovery and state
// topics owned by this publisher. MQTT has no list-topics operation, so retained
// wildcard subscriptions are the only way to discover records created by a
// previous process/version.
func (p *Publisher) loadRetainedInventory(ctx context.Context) error {
	if p.mqtt == nil || !p.mqtt.IsConnectionOpen() {
		return fmt.Errorf("mqtt connection is not open")
	}

	discoveryFilter := fmt.Sprintf("%s/+/omada_exporter/+/config", topicPrefix(p.client.Config.MQTTDiscoveryPrefix))
	stateFilter := fmt.Sprintf("%s/entities/+/state", topicPrefix(p.client.Config.MQTTTopicPrefix))
	trackerStateFilter := fmt.Sprintf("%s/device_trackers/+/state", topicPrefix(p.client.Config.MQTTTopicPrefix))
	trackerAttributesFilter := fmt.Sprintf("%s/device_trackers/+/attributes", topicPrefix(p.client.Config.MQTTTopicPrefix))
	retainedClientTrackers := map[string]clientTracker{}
	retainedClientAttributePayloads := map[string]retainedClientAttributes{}
	activity := make(chan struct{}, 1)

	notifyActivity := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}

	discoveryHandler := func(_ mqtt.Client, message mqtt.Message) {
		if !message.Retained() || len(message.Payload()) == 0 {
			return
		}

		var config struct {
			StateTopic string `json:"state_topic"`
			Origin     struct {
				Name string `json:"name"`
			} `json:"origin"`
		}
		if err := json.Unmarshal(message.Payload(), &config); err != nil || config.Origin.Name != "omada_exporter" {
			return
		}
		if !strings.HasPrefix(config.StateTopic, topicPrefix(p.client.Config.MQTTTopicPrefix)+"/entities/") {
			return
		}

		p.mu.Lock()
		p.retainedDiscovery[message.Topic()] = config.StateTopic
		p.mu.Unlock()
		notifyActivity()
	}

	stateHandler := func(_ mqtt.Client, message mqtt.Message) {
		if !message.Retained() || len(message.Payload()) == 0 {
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(message.Payload(), &payload); err != nil {
			return
		}
		labels := stringLabels(payload)
		if labels["metric"] == "" {
			return
		}

		p.mu.Lock()
		p.retainedStates[message.Topic()] = labels
		p.mu.Unlock()
		notifyActivity()
	}

	trackerStateHandler := func(_ mqtt.Client, message mqtt.Message) {
		if !message.Retained() || strings.TrimSpace(string(message.Payload())) != "home" {
			return
		}

		id, tracker, ok := retainedClientTracker(message.Topic(), p.client.Config.MQTTTopicPrefix)
		if !ok {
			return
		}

		p.mu.Lock()
		retainedClientTrackers[id] = tracker
		p.mu.Unlock()
		notifyActivity()
	}

	trackerAttributesHandler := func(_ mqtt.Client, message mqtt.Message) {
		if !message.Retained() || len(message.Payload()) == 0 {
			return
		}

		id, ok := retainedClientTrackerAttributesID(message.Topic(), p.client.Config.MQTTTopicPrefix)
		if !ok {
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(message.Payload(), &payload); err != nil {
			return
		}
		normalized, err := json.Marshal(payload)
		if err != nil {
			return
		}

		labels := stringLabels(payload)
		lastSeen, _ := time.Parse(time.RFC3339, labels["last_seen"])
		delete(labels, "last_seen")
		delete(labels, "last_updated")

		p.mu.Lock()
		retainedClientAttributePayloads[id] = retainedClientAttributes{
			Topic:    message.Topic(),
			Payload:  string(normalized),
			Labels:   labels,
			LastSeen: lastSeen,
		}
		p.mu.Unlock()
		notifyActivity()
	}

	for filter, handler := range map[string]mqtt.MessageHandler{
		discoveryFilter:         discoveryHandler,
		stateFilter:             stateHandler,
		trackerStateFilter:      trackerStateHandler,
		trackerAttributesFilter: trackerAttributesHandler,
	} {
		token := p.mqtt.Subscribe(filter, 0, handler)
		if !token.WaitTimeout(10 * time.Second) {
			return fmt.Errorf("subscribe to %s timed out", filter)
		}
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribe to %s: %w", filter, err)
		}
	}

	// Retained delivery has no end-of-snapshot marker. Wait until deliveries
	// have been quiet for one second, with a hard upper bound for large brokers.
	quiet := time.NewTimer(time.Second)
	maximum := time.NewTimer(10 * time.Second)
	defer quiet.Stop()
	defer maximum.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-activity:
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(time.Second)
		case <-quiet.C:
			goto snapshotComplete
		case <-maximum.C:
			goto snapshotComplete
		}
	}

snapshotComplete:
	if token := p.mqtt.Unsubscribe(discoveryFilter, stateFilter, trackerStateFilter, trackerAttributesFilter); token.WaitTimeout(10*time.Second) && token.Error() != nil {
		log.Warn().Err(token.Error()).Msg("failed to unsubscribe retained mqtt inventory filters")
	}
	p.mu.Lock()
	if p.lastClientTrackers == nil {
		p.lastClientTrackers = map[string]clientTracker{}
	}
	if p.clientLastSeen == nil {
		p.clientLastSeen = map[string]time.Time{}
	}
	if p.clientAttributes == nil {
		p.clientAttributes = map[string]string{}
	}
	for id, attributes := range retainedClientAttributePayloads {
		p.clientAttributes[attributes.Topic] = attributes.Payload
		if tracker, ok := retainedClientTrackers[id]; ok {
			tracker.Labels = copyLabels(attributes.Labels)
			retainedClientTrackers[id] = tracker
			p.lastClientTrackers[id] = tracker
			if !attributes.LastSeen.IsZero() {
				p.clientLastSeen[id] = attributes.LastSeen
			}
		}
	}
	for id, tracker := range retainedClientTrackers {
		p.knownClients[id] = tracker
	}
	p.retainedLoaded = true
	p.mu.Unlock()
	return nil
}

// retainedClientTracker reconstructs enough tracker metadata from a retained
// state topic to mark a previously-home dynamic client away after a restart.
func retainedClientTracker(topic, configuredPrefix string) (string, clientTracker, bool) {
	prefix := topicPrefix(configuredPrefix) + "/device_trackers/"
	if !strings.HasPrefix(topic, prefix) || !strings.HasSuffix(topic, "/state") {
		return "", clientTracker{}, false
	}

	id := strings.TrimSuffix(strings.TrimPrefix(topic, prefix), "/state")
	if id == "" || strings.Contains(id, "/") {
		return "", clientTracker{}, false
	}

	return id, clientTracker{
		StateTopic:      topic,
		AttributesTopic: prefix + id + "/attributes",
		Labels:          map[string]string{},
	}, true
}

func retainedClientTrackerAttributesID(topic, configuredPrefix string) (string, bool) {
	prefix := topicPrefix(configuredPrefix) + "/device_trackers/"
	if !strings.HasPrefix(topic, prefix) || !strings.HasSuffix(topic, "/attributes") {
		return "", false
	}

	id := strings.TrimSuffix(strings.TrimPrefix(topic, prefix), "/attributes")
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// stringLabels extracts the string-valued metric identity/attribute fields
// from a retained JSON state payload.
func stringLabels(payload map[string]any) map[string]string {
	labels := make(map[string]string, len(payload))
	for key, value := range payload {
		if text, ok := value.(string); ok {
			labels[key] = text
		}
	}
	return labels
}

// canonicalEntityTopics reuses the newest retained counterpart during an
// identity-policy migration. That preserves Home Assistant entity IDs,
// history, automations, and customizations while older duplicate counterparts
// are removed by reconciliation.
func (p *Publisher) canonicalEntityTopics(metricName string, labels map[string]string, component, fallbackObjectID, fallbackDiscoveryTopic, fallbackStateTopic string) (string, string, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.retainedLoaded {
		return fallbackObjectID, fallbackDiscoveryTopic, fallbackStateTopic
	}

	discoveryPrefix := fmt.Sprintf("%s/%s/omada_exporter/", topicPrefix(p.client.Config.MQTTDiscoveryPrefix), component)
	wantedKey := supersessionKey(metricName, labels)
	bestDiscoveryTopic := ""
	bestStateTopic := ""
	bestObjectID := ""
	bestObservedAt := time.Time{}

	for discoveryTopic, stateTopic := range p.retainedDiscovery {
		if !strings.HasPrefix(discoveryTopic, discoveryPrefix) || !strings.HasSuffix(discoveryTopic, "/config") {
			continue
		}
		retainedLabels := p.retainedStates[stateTopic]
		if retainedLabels["metric"] != metricName || supersessionKey(metricName, retainedLabels) != wantedKey || !sameLogicalSite(retainedLabels, labels) {
			continue
		}

		objectID := strings.TrimSuffix(strings.TrimPrefix(discoveryTopic, discoveryPrefix), "/config")
		if objectID == "" || strings.Contains(objectID, "/") {
			continue
		}
		observedAt, _ := time.Parse(time.RFC3339, retainedLabels["last_updated"])
		if bestDiscoveryTopic == "" || observedAt.After(bestObservedAt) || (observedAt.Equal(bestObservedAt) && discoveryTopic < bestDiscoveryTopic) {
			bestDiscoveryTopic = discoveryTopic
			bestStateTopic = stateTopic
			bestObjectID = objectID
			bestObservedAt = observedAt
		}
	}

	if bestDiscoveryTopic == "" {
		return fallbackObjectID, fallbackDiscoveryTopic, fallbackStateTopic
	}
	return bestObjectID, bestDiscoveryTopic, bestStateTopic
}

// reconcileSupersededEntities removes only records for which a current entity
// with the same semantic identity exists. Missing entities without a confirmed
// counterpart are deliberately preserved, so API failures and offline clients
// cannot trigger broad deletion.
func (p *Publisher) reconcileSupersededEntities(current map[string]entity) {
	if !p.client.Config.MQTTRetain {
		return
	}

	currentByKey := make(map[string]entity, len(current))
	for _, ent := range current {
		currentByKey[supersessionKey(ent.MetricName, ent.Labels)] = ent
	}

	p.mu.Lock()
	for topic, ent := range current {
		p.retainedDiscovery[topic] = ent.StateTopic
		labels := copyLabels(ent.Labels)
		labels["metric"] = ent.MetricName
		p.retainedStates[ent.StateTopic] = labels
	}
	loaded := p.retainedLoaded
	knownDiscovery := make(map[string]string, len(p.retainedDiscovery))
	for topic, stateTopic := range p.retainedDiscovery {
		knownDiscovery[topic] = stateTopic
	}
	knownStates := make(map[string]map[string]string, len(p.retainedStates))
	for topic, labels := range p.retainedStates {
		knownStates[topic] = copyLabels(labels)
	}
	p.mu.Unlock()

	if !loaded {
		return
	}

	for discoveryTopic, stateTopic := range knownDiscovery {
		if _, isCurrent := current[discoveryTopic]; isCurrent {
			continue
		}
		oldLabels := knownStates[stateTopic]
		metricName := oldLabels["metric"]
		if metricName == "" {
			continue
		}
		counterpart, ok := currentByKey[supersessionKey(metricName, oldLabels)]
		if !ok || !sameLogicalSite(oldLabels, counterpart.Labels) {
			continue
		}

		discoveryRemoved := p.publishBytes(discoveryTopic, []byte{}, true)
		stateRemoved := p.publishBytes(stateTopic, []byte{}, true)
		if !discoveryRemoved || !stateRemoved {
			log.Warn().
				Str("topic", discoveryTopic).
				Str("state_topic", stateTopic).
				Str("metric", metricName).
				Bool("discovery_removed", discoveryRemoved).
				Bool("state_removed", stateRemoved).
				Msg("failed to fully remove superseded retained mqtt entity; cleanup will be retried")
			continue
		}
		log.Info().Str("topic", discoveryTopic).Str("metric", metricName).Msg("removed superseded retained mqtt entity")

		p.mu.Lock()
		delete(p.retainedDiscovery, discoveryTopic)
		delete(p.retainedStates, stateTopic)
		delete(p.published, discoveryTopic)
		p.mu.Unlock()
	}
}

func supersessionKey(metricName string, labels map[string]string) string {
	return strings.Join(metricIdentityParts(metricName, labels, false), "\x00")
}

// sameLogicalSite accepts an exact site ID or exact site name match. This
// permits safe reconciliation across a controller migration that regenerated
// site IDs, while refusing deletion when both site identity fields changed.
func sameLogicalSite(oldLabels, currentLabels map[string]string) bool {
	oldID := strings.TrimSpace(oldLabels["site_id"])
	currentID := strings.TrimSpace(currentLabels["site_id"])
	oldName := strings.TrimSpace(oldLabels["site"])
	currentName := strings.TrimSpace(currentLabels["site"])

	if oldID != "" && currentID != "" && oldID == currentID {
		return true
	}
	if oldName != "" && currentName != "" && oldName == currentName {
		return true
	}
	return oldID == "" && currentID == "" && oldName == "" && currentName == ""
}

// metricValue extracts a numeric value from a Prometheus metric.
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

// metricPayloadValue converts a metric value into a JSON-friendly payload.
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

// metricLabels builds a label map from a Prometheus metric.
func metricLabels(metric *dto.Metric) map[string]string {
	labels := make(map[string]string, len(metric.Label))
	for _, label := range metric.Label {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}

// isBinaryMetric reports whether the metric should be published as a binary sensor.
func isBinaryMetric(name string) bool {
	switch name {
	case "omada_controller_upgrade_available",
		"omada_device_need_upgrade",
		"omada_port_link_status",
		"omada_lag_link_status",
		"omada_isp_status",
		"omada_vpn_status",
		"omada_site_to_site_vpn_peer_status":
		return true
	default:
		return false
	}
}

// binaryDeviceClass returns the Home Assistant device class for a binary metric.
func binaryDeviceClass(name string) string {
	switch name {
	case "omada_controller_upgrade_available", "omada_device_need_upgrade":
		return "problem"
	case "omada_port_link_status", "omada_lag_link_status", "omada_isp_status", "omada_vpn_status", "omada_site_to_site_vpn_peer_status":
		return "connectivity"
	default:
		return ""
	}
}

// sensorHints returns Home Assistant metadata hints for a metric.
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

// friendlyMetricName builds a readable entity name for a metric.
func friendlyMetricName(metricName string, labels map[string]string) string {
	base := strings.TrimPrefix(metricName, "omada_")
	parts := strings.Split(base, "_")
	for i, part := range parts {
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	name := strings.Join(parts, " ")

	qualifiers := []string{}
	for _, key := range friendlyQualifierLabels(metricName) {
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

// friendlyQualifierLabels selects display qualifiers that belong to the
// metric itself. Client attachment labels are deliberately excluded: moving a
// client must update attributes without renaming the entity.
func friendlyQualifierLabels(metricName string) []string {
	switch {
	case strings.HasPrefix(metricName, "omada_controller_storage_"):
		return []string{"storage_name"}
	case metricName == "omada_controller_upgrade_available":
		return []string{"upgrade_channel"}
	case strings.HasPrefix(metricName, "omada_port_"):
		return []string{"port"}
	case strings.HasPrefix(metricName, "omada_lag_"):
		return []string{"lag_id"}
	case strings.HasPrefix(metricName, "omada_wan_"), strings.HasPrefix(metricName, "omada_isp_"):
		return []string{"port", "name"}
	case metricName == "omada_client_connected_total":
		return []string{"connection_mode", "wifi_mode"}
	case strings.HasPrefix(metricName, "omada_vpn_"), strings.HasPrefix(metricName, "omada_site_to_site_vpn_"):
		return []string{"name", "peer_name"}
	default:
		return nil
	}
}

// objectID builds a stable Home Assistant object identifier for a metric.
func objectID(metricName string, labels map[string]string) string {
	stable := metricIdentityParts(metricName, labels, true)
	return slug(strings.Join(stable, "_")) + "_" + shortHash(stable)
}

// metricIdentityParts returns only labels that identify the metric's owning
// object or real subresource. Mutable names, client attachment paths, and live
// topology properties must remain attributes rather than entity identity.
func metricIdentityParts(metricName string, labels map[string]string, includeSite bool) []string {
	parts := []string{metricName}
	appendIdentity := func(key string) {
		if value := strings.TrimSpace(labels[key]); value != "" {
			parts = append(parts, key+"_"+value)
		}
	}

	if includeSite {
		if strings.TrimSpace(labels["site_id"]) != "" {
			appendIdentity("site_id")
		} else {
			appendIdentity("site")
		}
	}

	hasOwner := false
	for _, key := range []string{"device_mac", "mac", "gateway_mac"} {
		if strings.TrimSpace(labels[key]) != "" {
			appendIdentity(key)
			hasOwner = true
			break
		}
	}

	switch {
	case strings.HasPrefix(metricName, "omada_controller_storage_"):
		appendIdentity("storage_name")
	case metricName == "omada_controller_upgrade_available":
		appendIdentity("upgrade_channel")
	case strings.HasPrefix(metricName, "omada_port_"), strings.HasPrefix(metricName, "omada_wan_"), strings.HasPrefix(metricName, "omada_isp_"):
		appendIdentity("port")
	case strings.HasPrefix(metricName, "omada_lag_"):
		appendIdentity("lag_id")
	case metricName == "omada_client_connected_total":
		appendIdentity("connection_mode")
		appendIdentity("wifi_mode")
	case metricName == "omada_dpi_category_traffic_bytes":
		appendIdentity("family_id")
		if strings.TrimSpace(labels["family_id"]) == "" {
			appendIdentity("family_name")
		}
	case metricName == "omada_dpi_application_traffic_bytes":
		appendIdentity("family_id")
		appendIdentity("application_id")
		if strings.TrimSpace(labels["application_id"]) == "" {
			appendIdentity("application_name")
		}
	}

	vpnID := strings.TrimSpace(labels["vpn_id"])
	if vpnID != "" && (metricName == "omada_vpn_status" || strings.HasPrefix(metricName, "omada_site_to_site_vpn_")) {
		appendIdentity("vpn_id")
		if strings.HasPrefix(metricName, "omada_site_to_site_vpn_peer_") {
			if strings.TrimSpace(labels["peer_id"]) != "" {
				appendIdentity("peer_id")
			} else {
				appendIdentity("peer_name")
				appendIdentity("remote_ip")
			}
		}
		return parts
	}

	if strings.HasPrefix(metricName, "omada_vpn_") && vpnID == "" {
		for _, key := range []string{"name", "interface_name", "vpn_mode", "vpn_type"} {
			appendIdentity(key)
		}
		return parts
	}

	if !hasOwner && len(parts) == 1 {
		for _, key := range []string{"interface_name", "connection_mode", "wifi_mode", "name"} {
			appendIdentity(key)
		}
	}

	return parts
}

// shortHash returns a short hash for the provided values.
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

// slug converts a string into a Home Assistant safe slug.
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

// topicPrefix normalizes the MQTT topic prefix.
func topicPrefix(prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "omada_exporter"
	}
	return prefix
}

// normalizeBroker normalizes the MQTT broker address.
func normalizeBroker(broker string) string {
	if strings.Contains(broker, "://") {
		return broker
	}
	return "tcp://" + broker
}

// deviceInfo builds Home Assistant device metadata for a metric.
func deviceInfo(client *api.Client, metricName string, labels map[string]string) map[string]any {
	if strings.HasPrefix(metricName, "omada_client_") && labels["mac"] != "" && labels["device_mac"] == "" {
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

// compactDevice removes empty values from Home Assistant device metadata.
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

// clientName returns the display name for a client label set.
func clientName(labels map[string]string) string {
	return firstNonEmpty(labels["name"], labels["host_name"], labels["system_name"], labels["ip"], labels["mac"], "Omada Client")
}

// parseTrackedClientMACs parses the configured MAC address list for forced MQTT client trackers.
func parseTrackedClientMACs(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})

	macs := []string{}
	seen := map[string]struct{}{}
	for _, part := range parts {
		mac, ok := normalizeClientMAC(part)
		if !ok {
			log.Warn().Str("mac", part).Msg("ignoring invalid tracked client mac")
			continue
		}

		id := trackerID(mac)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		macs = append(macs, mac)
	}
	return macs
}

// normalizeClientMAC normalizes common MAC address spellings to aa:bb:cc:dd:ee:ff.
func normalizeClientMAC(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", false
	}

	var raw strings.Builder
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			raw.WriteRune(r)
		case r >= 'a' && r <= 'f':
			raw.WriteRune(r)
		case r == ':' || r == '-' || r == '.' || r == '_':
			continue
		default:
			return "", false
		}
	}

	hex := raw.String()
	if len(hex) != 12 {
		return "", false
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s", hex[0:2], hex[2:4], hex[4:6], hex[6:8], hex[8:10], hex[10:12]), true
}

// trackerID builds the tracker identifier for a client MAC address.
func trackerID(mac string) string {
	return slug(strings.ReplaceAll(strings.ToLower(mac), ":", "_"))
}

// firstNonEmpty returns the first non-empty string in the provided values.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// copyLabels returns a shallow copy of a metric label map.
func copyLabels(labels map[string]string) map[string]string {
	copied := make(map[string]string, len(labels))
	for key, value := range labels {
		copied[key] = value
	}
	return copied
}

// copyClientTrackers returns a shallow copy of tracked client metadata.
func copyClientTrackers(trackers map[string]clientTracker) map[string]clientTracker {
	copied := make(map[string]clientTracker, len(trackers))
	for id, tracker := range trackers {
		tracker.Labels = copyLabels(tracker.Labels)
		copied[id] = tracker
	}
	return copied
}

// buildPublishContext builds lookup data used while publishing related metrics.
func buildPublishContext(families []*dto.MetricFamily) publishContext {
	modeTypeNameCounts := map[string]int{}
	modeTypeNameIDs := map[string]string{}
	nameCounts := map[string]int{}
	nameIDs := map[string]string{}
	infrastructureByMAC := map[string]map[string]string{}

	for _, family := range families {
		for _, metric := range family.Metric {
			labels := metricLabels(metric)
			if (strings.HasPrefix(family.GetName(), "omada_controller_") || strings.HasPrefix(family.GetName(), "omada_device_")) && labels["device_mac"] != "" {
				id := trackerID(labels["device_mac"])
				existing := infrastructureByMAC[id]
				if existing == nil {
					existing = map[string]string{}
					infrastructureByMAC[id] = existing
				}
				for key, value := range labels {
					if value != "" {
						existing[key] = value
					}
				}
			}

			if family.GetName() != "omada_vpn_status" {
				continue
			}
			vpnID := strings.TrimSpace(labels["vpn_id"])
			if vpnID == "" {
				continue
			}

			modeTypeNameKey := vpnLookupKey(labels["name"], labels["vpn_mode"], labels["vpn_type"])
			if modeTypeNameKey != "" {
				modeTypeNameCounts[modeTypeNameKey]++
				modeTypeNameIDs[modeTypeNameKey] = vpnID
			}

			nameKey := slug(labels["name"])
			if nameKey != "" {
				nameCounts[nameKey]++
				nameIDs[nameKey] = vpnID
			}
		}
	}

	return publishContext{
		vpnIDByModeTypeName: uniqueLookup(modeTypeNameCounts, modeTypeNameIDs),
		vpnIDByName:         uniqueLookup(nameCounts, nameIDs),
		infrastructureByMAC: infrastructureByMAC,
	}
}

// uniqueLookup returns values that are unique within the provided counts.
func uniqueLookup(counts map[string]int, values map[string]string) map[string]string {
	lookup := make(map[string]string, len(values))
	for key, value := range values {
		if counts[key] == 1 && value != "" {
			lookup[key] = value
		}
	}
	return lookup
}

// deviceLabels returns the labels used to identify the owning device for a metric.
func deviceLabels(metricName string, labels map[string]string, ctx publishContext) map[string]string {
	if strings.HasPrefix(metricName, "omada_client_") && labels["mac"] != "" {
		if infrastructure := ctx.infrastructureByMAC[trackerID(labels["mac"])]; infrastructure != nil {
			enriched := copyLabels(labels)
			for key, value := range infrastructure {
				enriched[key] = value
			}
			return enriched
		}
	}

	if !strings.HasPrefix(metricName, "omada_vpn_") || labels["vpn_id"] != "" {
		return labels
	}

	vpnID := ctx.vpnIDByModeTypeName[vpnLookupKey(labels["name"], labels["vpn_mode"], labels["vpn_type"])]
	if vpnID == "" {
		vpnID = ctx.vpnIDByName[slug(labels["name"])]
	}
	if vpnID == "" {
		return labels
	}

	enriched := copyLabels(labels)
	enriched["vpn_id"] = vpnID
	return enriched
}

// vpnLookupKey builds a lookup key for VPN-related metrics.
func vpnLookupKey(name, mode, vpnType string) string {
	parts := []string{slug(name), slug(mode), slug(vpnType)}
	if parts[0] == "" {
		return ""
	}
	return strings.Join(parts, "|")
}

// derivedMetric returns the derived metric definition for a source metric.
func derivedMetric(metricName string) (string, string, bool) {
	switch metricName {
	case "omada_vpn_down_bytes":
		return "omada_vpn_down_speed", "VPN downlink speed in bits per second", true
	case "omada_vpn_up_bytes":
		return "omada_vpn_up_speed", "VPN uplink speed in bits per second", true
	case "omada_site_to_site_vpn_down_bytes":
		return "omada_site_to_site_vpn_down_speed", "Site-to-site VPN downlink speed in bits per second", true
	case "omada_site_to_site_vpn_up_bytes":
		return "omada_site_to_site_vpn_up_speed", "Site-to-site VPN uplink speed in bits per second", true
	case "omada_site_to_site_vpn_peer_down_bytes":
		return "omada_site_to_site_vpn_peer_down_speed", "Site-to-site VPN peer downlink speed in bits per second", true
	case "omada_site_to_site_vpn_peer_up_bytes":
		return "omada_site_to_site_vpn_peer_up_speed", "Site-to-site VPN peer uplink speed in bits per second", true
	default:
		return "", "", false
	}
}

// recordRateSample stores a metric sample and returns its calculated rate.
func (p *Publisher) recordRateSample(sampleKey string, value float64, observedAt time.Time) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	previous, ok := p.metricSamples[sampleKey]
	p.metricSamples[sampleKey] = metricSample{
		Value:      value,
		ObservedAt: observedAt,
	}

	if !ok || !observedAt.After(previous.ObservedAt) || value < previous.Value {
		return 0
	}

	deltaSeconds := observedAt.Sub(previous.ObservedAt).Seconds()
	if deltaSeconds <= 0 {
		return 0
	}

	return (value - previous.Value) * 8 / deltaSeconds
}
