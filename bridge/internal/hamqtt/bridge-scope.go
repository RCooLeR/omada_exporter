package hamqtt

import (
	"strings"

	"github.com/RCooLeR/omada_exporter/internal/api"
)

// Empty IDs deliberately retain every legacy topic and identifier. Named
// bridges use a sibling prefix, never a subtree of the legacy prefix.
func scopedValue(value, bridgeID string) string {
	if bridgeID == "" {
		return value
	}
	return value + "_" + bridgeID
}

func (p *Publisher) statePrefix() string {
	return scopedValue(topicPrefix(p.client.Config.MQTTTopicPrefix), p.client.Config.BridgeID)
}

func (p *Publisher) discoveryNode() string {
	return scopedValue("omada_exporter", p.client.Config.BridgeID)
}

func (p *Publisher) scopedLabels(labels map[string]string) map[string]string {
	result := copyLabels(labels)
	delete(result, "bridge_id")
	if p.client.Config.BridgeID != "" {
		result["bridge_id"] = p.client.Config.BridgeID
		// Sparse/configured trackers must join the same site as live observations.
		if result["site_id"] == "" && (result["site"] == "" || result["site"] == p.client.Config.Site) {
			_, siteID := p.client.ContextIDs()
			if siteID != "" {
				result["site_id"] = siteID
			}
		}
	}
	return result
}

func logicalSiteID(labels map[string]string) string {
	if id := strings.TrimSpace(labels["site_id"]); id != "" {
		return "id:" + id
	}
	return "name:" + strings.TrimSpace(labels["site"])
}

func scopedDeviceID(client *api.Client, base string, labels map[string]string) string {
	if client.Config.BridgeID == "" {
		return base
	}
	return scopedValue(base, client.Config.BridgeID) + "_" + shortHash([]string{logicalSiteID(labels)})
}

func (p *Publisher) clientTrackerID(labels map[string]string) string {
	return scopedDeviceID(p.client, trackerID(labels["mac"]), p.scopedLabels(labels))
}

func (p *Publisher) ownsLabels(labels map[string]string) bool {
	return labels["bridge_id"] == p.client.Config.BridgeID
}

func (p *Publisher) ownsEntityState(topic string) bool {
	parts := strings.Split(strings.TrimPrefix(topic, p.statePrefix()+"/"), "/")
	return strings.HasPrefix(topic, p.statePrefix()+"/") && len(parts) == 3 && parts[0] == "entities" && parts[1] != "" && parts[2] == "state"
}

func (p *Publisher) ownsDiscovery(topic string) bool {
	if p.client.Config.BridgeID == "" && p.client.Config.MQTTDiscoveryPrefix == "" {
		// Legacy zero-value callers supplied inventory independently of a
		// discovery prefix. Still require the exact legacy node boundary.
		parts := strings.Split(topic, "/")
		return len(parts) >= 4 && parts[len(parts)-3] == p.discoveryNode() && parts[len(parts)-2] != "" && parts[len(parts)-1] == "config"
	}
	prefix := topicPrefix(p.client.Config.MQTTDiscoveryPrefix) + "/"
	parts := strings.Split(strings.TrimPrefix(topic, prefix), "/")
	return strings.HasPrefix(topic, prefix) && len(parts) == 4 && parts[0] != "" && parts[1] == p.discoveryNode() && parts[2] != "" && parts[3] == "config"
}
