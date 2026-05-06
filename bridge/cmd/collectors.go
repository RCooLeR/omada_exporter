package cmd

import (
	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/collectors"
	"github.com/prometheus/client_golang/prometheus"
)

// initCollectors builds the Prometheus collectors map for the API client.
func initCollectors(client *api.Client) map[string]prometheus.Collector {
	return map[string]prometheus.Collector{
		"controller": collector.NewControllerCollector(client),
		"alert":      collector.NewAlertCollector(client),
		"device":     collector.NewDeviceCollector(client),
		"client":     collector.NewClientCollector(client),
		"vpn":        collector.NewVpnCollector(client),
		"vpn-stats":  collector.NewVpnStatsCollector(client),
		"isp":        collector.NewISPCollector(client),
	}
}
