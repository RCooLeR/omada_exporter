package cmd

import (
	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/collectors"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

// initCollectors builds the Prometheus collectors map for the API client.
// A nil client returns every collector, for metric documentation.
func initCollectors(client *api.Client) map[string]prometheus.Collector {
	collectors := map[string]prometheus.Collector{
		"controller": collector.NewControllerCollector(client),
		"alert":      collector.NewAlertCollector(client),
		"device":     collector.NewDeviceCollector(client),
	}
	// These read only the Open API. Without credentials they would fail and log on every scrape.
	if client == nil || client.OpenAPIAuthMode != config.OpenAPIAuthDisabled {
		collectors["client"] = collector.NewClientCollector(client)
		collectors["vpn"] = collector.NewVpnCollector(client)
		collectors["vpn-stats"] = collector.NewVpnStatsCollector(client)
		collectors["isp"] = collector.NewISPCollector(client)
	}
	if client == nil || client.Config == nil || client.Config.TrackInsightMetrics {
		collectors["insights"] = collector.NewInsightsCollector(client)
	}
	if client == nil || client.Config == nil || client.Config.TrackNetworkQualityMetrics {
		collectors["network-quality"] = collector.NewNetworkQualityCollector(client)
	}
	return collectors
}
