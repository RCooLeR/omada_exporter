package collector

import (
	"math"
	"strings"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

// NetworkQualityCollector reads optional OpenAPI telemetry independently of the
// device collector: an unsupported endpoint must not hide ordinary device data.
type NetworkQualityCollector struct {
	client                             *omada.Client
	getDevices                         func() ([]model.DevicesInterface, error)
	getAPUplink                        func(string) (*model.APUplinkStats, error)
	getWANQuality                      func(string, int) (*model.WANQuality, error)
	packets, drops, errors             *prometheus.Desc
	latency, jitter, loss, mos, window *prometheus.Desc
	success, available                 *prometheus.Desc
}

// NewNetworkQualityCollector builds descriptors without fetching API data.
func NewNetworkQualityCollector(client *omada.Client) *NetworkQualityCollector {
	labels := []string{"device_mac", "device_name", "device_type", "site", "site_id"}
	packetLabels := append(append([]string{}, labels...), "direction")
	statusLabels := append(append([]string{}, labels...), "source")
	desc := func(name, help string, labels []string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &NetworkQualityCollector{
		client: client, getDevices: client.GetDevices, getAPUplink: client.GetAPUplinkStats, getWANQuality: client.GetWANQuality,
		packets:   desc("omada_ap_uplink_packets_total", "Controller-reported wired AP uplink packet counter; up/down follow Omada direction. May reset on device/controller restart or counter reset.", packetLabels),
		drops:     desc("omada_ap_uplink_dropped_packets_total", "Controller-reported wired AP uplink dropped-packet counter; up/down follow Omada direction. May reset on device/controller restart or counter reset.", packetLabels),
		errors:    desc("omada_ap_uplink_error_packets_total", "Controller-reported wired AP uplink error-packet counter; up/down follow Omada direction. May reset on device/controller restart or counter reset.", packetLabels),
		latency:   desc("omada_gateway_wan_quality_latency_average_seconds", "Gateway-wide WAN latency average over the configured rolling quality window, in seconds; not instantaneous or per-interface.", labels),
		jitter:    desc("omada_gateway_wan_quality_jitter_average_seconds", "Gateway-wide WAN jitter average over the configured rolling quality window, in seconds; not instantaneous or per-interface.", labels),
		loss:      desc("omada_gateway_wan_quality_packet_loss_average_percentage", "Gateway-wide WAN packet-loss percentage averaged over the configured rolling quality window; not a cumulative counter.", labels),
		mos:       desc("omada_gateway_wan_quality_mos_average", "Gateway-wide mean opinion score averaged over the configured rolling quality window.", labels),
		window:    desc("omada_gateway_wan_quality_window_seconds", "Configured rolling query window for gateway WAN quality averages, in seconds.", labels),
		success:   desc("omada_network_quality_scrape_success", "Whether the attempted network-quality API query succeeded. Successful unsupported or empty responses are distinguished by the available metric.", statusLabels),
		available: desc("omada_network_quality_available", "Whether at least one valid network-quality measurement was returned for this device/source; 0 for offline, unsupported, empty or failed queries.", statusLabels),
	}
}

func (c *NetworkQualityCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{c.packets, c.drops, c.errors, c.latency, c.jitter, c.loss, c.mos, c.window, c.success, c.available} {
		ch <- desc
	}
}

func (c *NetworkQualityCollector) Collect(ch chan<- prometheus.Metric) {
	if c.client == nil || c.client.Settings() == nil || !c.client.Settings().TrackNetworkQualityMetrics {
		return
	}
	devices, err := c.getDevices()
	_, siteID := c.client.ContextIDs()
	inventoryLabels := []string{"", "", "", c.client.Settings().Site, siteID, "inventory"}
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, boolFloat64(err == nil), inventoryLabels...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get network-quality device inventory")
		return
	}
	var group errgroup.Group
	group.SetLimit(4)
	for _, device := range devices {
		group.Go(func() error {
			switch d := device.(type) {
			case *model.AccessPoint:
				c.collectAP(ch, d, siteID)
			case *model.Gateway:
				c.collectWAN(ch, d, siteID)
			}
			return nil
		})
	}
	_ = group.Wait()
}

func (c *NetworkQualityCollector) labels(device model.DeviceInterface, siteID string) []string {
	return []string{device.GetMac(), device.GetName(), device.GetType(), c.client.Settings().Site, siteID}
}

func (c *NetworkQualityCollector) collectAP(ch chan<- prometheus.Metric, ap *model.AccessPoint, siteID string) {
	labels := c.labels(ap, siteID)
	statusLabels := append(append([]string{}, labels...), "ap_uplink")
	available := false
	defer func() {
		ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, boolFloat64(available), statusLabels...)
	}()
	// A wireless-backhaul AP has no wired uplink even when its radios are online.
	if !strings.HasPrefix(ap.GetStatus(), "Connected") || ap.WirelessLinked || strings.HasPrefix(ap.GetStatus(), "Connected(Wireless") {
		return
	}
	stats, err := c.getAPUplink(ap.GetMac())
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, boolFloat64(err == nil), statusLabels...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get AP uplink quality")
		return
	}
	if stats == nil {
		return
	}
	for _, item := range []struct {
		desc     *prometheus.Desc
		up, down *uint64
	}{
		{c.packets, stats.UpPackets, stats.DownPackets}, {c.drops, stats.UpDropPackets, stats.DownDropPackets}, {c.errors, stats.UpErrorPackets, stats.DownErrorPackets},
	} {
		for _, direction := range []struct {
			name  string
			value *uint64
		}{{"up", item.up}, {"down", item.down}} {
			if direction.value != nil {
				available = true
				ch <- prometheus.MustNewConstMetric(item.desc, prometheus.CounterValue, float64(*direction.value), append(append([]string{}, labels...), direction.name)...)
			}
		}
	}
}

func (c *NetworkQualityCollector) collectWAN(ch chan<- prometheus.Metric, gateway *model.Gateway, siteID string) {
	labels := c.labels(gateway, siteID)
	statusLabels := append(append([]string{}, labels...), "wan")
	available := false
	defer func() {
		ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, boolFloat64(available), statusLabels...)
	}()
	if !strings.HasPrefix(gateway.GetStatus(), "Connected") {
		return
	}
	window := c.client.Settings().NetworkQualityWindowSeconds
	if window <= 0 {
		window = 86400
	}
	quality, err := c.getWANQuality(gateway.GetMac(), window)
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, boolFloat64(err == nil), statusLabels...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get WAN quality")
		return
	}
	if quality == nil {
		return
	}
	for _, item := range []struct {
		desc       *prometheus.Desc
		value      *float64
		scale, max float64
	}{
		{c.latency, quality.LatencyMilliseconds, 0.001, math.MaxFloat64},
		{c.jitter, quality.JitterMilliseconds, 0.001, math.MaxFloat64},
		{c.loss, quality.PacketLossPercent, 1, 100},
		{c.mos, quality.MOS, 1, 5},
	} {
		if validQualityValue(item.value, item.max) {
			available = true
			ch <- prometheus.MustNewConstMetric(item.desc, prometheus.GaugeValue, *item.value*item.scale, labels...)
		}
	}
	if available {
		ch <- prometheus.MustNewConstMetric(c.window, prometheus.GaugeValue, float64(window), labels...)
	}
}

func validQualityValue(value *float64, max float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= max
}
