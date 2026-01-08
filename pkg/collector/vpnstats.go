package collector

import (
	"strconv"
	"strings"

	"github.com/RCooLeR/omada_exporter/pkg/api"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type vpnStatsCollector struct {
	omadaVpnUptime    *prometheus.Desc
	omadaVpnDownBytes *prometheus.Desc
	omadaVpnUpBytes   *prometheus.Desc
	client            *api.Client
}

func (c *vpnStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaVpnUptime
	ch <- c.omadaVpnDownBytes
	ch <- c.omadaVpnUpBytes
}
func parseDurationToMinutes(duration string) int {
	totalMinutes := 0
	parts := strings.Fields(duration)

	for _, part := range parts {
		if strings.HasSuffix(part, "d") {
			days, _ := strconv.Atoi(strings.TrimSuffix(part, "d"))
			totalMinutes += days * 24 * 60
		} else if strings.HasSuffix(part, "h") {
			hours, _ := strconv.Atoi(strings.TrimSuffix(part, "h"))
			totalMinutes += hours * 60
		} else if strings.HasSuffix(part, "m") {
			minutes, _ := strconv.Atoi(strings.TrimSuffix(part, "m"))
			totalMinutes += minutes
		}
	}

	return totalMinutes
}
func (c *vpnStatsCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config

	site := config.Site
	vpn, err := client.GetVpnStats()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get VPN stats")
		return
	}

	for _, item := range vpn {
		vpnmode := getVpnMode(item.VpnMode)
		vpntype := getVpnType(item.VpnType)
		uptime := parseDurationToMinutes(item.Uptime)
		labels := []string{item.Name, item.InterfaceName, vpnmode, vpntype, item.LocalIp, item.RemoteIp, site, client.SiteId}
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUptime, prometheus.GaugeValue, float64(uptime), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnDownBytes, prometheus.GaugeValue, float64(item.DownBytes), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUpBytes, prometheus.GaugeValue, float64(item.UpBytes), labels...)
	}
}

func NewVpnStatsCollector(c *api.Client) *vpnStatsCollector {
	labels := []string{"name", "interface_name", "vpn_mode", "vpn_type", "local_ip", "remote_ip", "site", "site_id"}

	return &vpnStatsCollector{
		omadaVpnUptime: prometheus.NewDesc("omada_vpn_uptime",
			"The current uptime of the VPN",
			labels,
			nil,
		),
		omadaVpnDownBytes: prometheus.NewDesc("omada_vpn_down_bytes",
			"VPN downlink traffic in bytes",
			labels,
			nil,
		),
		omadaVpnUpBytes: prometheus.NewDesc("omada_vpn_up_bytes",
			"VPN uplink traffic in bytes",
			labels,
			nil,
		),
		client: c,
	}

}
