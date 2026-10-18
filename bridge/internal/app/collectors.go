package app

import (
	"github.com/RCooLeR/omada_exporter/internal/collectors"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/prometheus/client_golang/prometheus"
)

// initCollectors builds the Prometheus collectors map for the API client.
// A nil client includes all descriptors for offline metric documentation.
func initCollectors(client *omada.Client) map[string]prometheus.Collector {
	collectors := map[string]prometheus.Collector{
		"controller": collector.NewControllerCollector(client),
		"alert":      collector.NewAlertCollector(client),
		"device":     collector.NewDeviceCollector(client),
	}
	// Use the resolved mode: Fusion can authenticate without app credentials.
	openAPIEnabled := client == nil || client.OpenAPIEnabled()
	if openAPIEnabled {
		collectors["client"] = collector.NewClientCollector(client)
		collectors["vpn"] = collector.NewVpnCollector(client)
		collectors["vpn-stats"] = collector.NewVpnStatsCollector(client)
		collectors["isp"] = collector.NewISPCollector(client)
	}
	if client == nil || client.Settings() == nil || client.Settings().TrackInsightMetrics {
		collectors["insights"] = collector.NewInsightsCollector(client)
	}
	if openAPIEnabled && (client == nil || client.Settings() == nil || client.Settings().TrackNetworkQualityMetrics) {
		collectors["network-quality"] = collector.NewNetworkQualityCollector(client)
	}
	return collectors
}
