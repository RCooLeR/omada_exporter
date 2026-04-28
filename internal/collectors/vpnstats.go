package collector

import (
	"strconv"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/openapi"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

// vpnStatsCollector collects and exports VPN stats metrics.
type vpnStatsCollector struct {
	omadaVpnUptime                       *prometheus.Desc
	omadaVpnDownPackets                  *prometheus.Desc
	omadaVpnDownBytes                    *prometheus.Desc
	omadaVpnUpPackets                    *prometheus.Desc
	omadaVpnUpBytes                      *prometheus.Desc
	omadaSiteToSiteVpnConnectedPeers     *prometheus.Desc
	omadaSiteToSiteVpnDisconnectedPeers  *prometheus.Desc
	omadaSiteToSiteVpnTotalPeers         *prometheus.Desc
	omadaSiteToSiteVpnPeerStatus         *prometheus.Desc
	omadaSiteToSiteVpnPeerDownBytes      *prometheus.Desc
	omadaSiteToSiteVpnPeerUpBytes        *prometheus.Desc
	omadaSiteToSiteVpnPeerDownPackets    *prometheus.Desc
	omadaSiteToSiteVpnPeerUpPackets      *prometheus.Desc
	omadaSiteToSiteVpnPeerLoginTimestamp *prometheus.Desc
	client                               *openapi.Client
}

// Describe sends the collector metric descriptors to Prometheus.
func (c *vpnStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaVpnUptime
	ch <- c.omadaVpnDownPackets
	ch <- c.omadaVpnDownBytes
	ch <- c.omadaVpnUpPackets
	ch <- c.omadaVpnUpBytes
	ch <- c.omadaSiteToSiteVpnConnectedPeers
	ch <- c.omadaSiteToSiteVpnDisconnectedPeers
	ch <- c.omadaSiteToSiteVpnTotalPeers
	ch <- c.omadaSiteToSiteVpnPeerStatus
	ch <- c.omadaSiteToSiteVpnPeerDownBytes
	ch <- c.omadaSiteToSiteVpnPeerUpBytes
	ch <- c.omadaSiteToSiteVpnPeerDownPackets
	ch <- c.omadaSiteToSiteVpnPeerUpPackets
	ch <- c.omadaSiteToSiteVpnPeerLoginTimestamp
}

// Collect fetches current data and emits Prometheus metrics.
func (c *vpnStatsCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	site := client.Config.Site

	vpn, err := client.GetVpnStats()
	seenPacketSeries := map[string]struct{}{}
	if err != nil {
		log.Error().Err(err).Msg("Failed to get VPN stats")
	} else {
		seenPacketSeries = c.collectVpnTunnelMetrics(ch, site, vpn)
	}

	summaries, err := client.GetSiteToSiteVpnSummaries()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get site-to-site VPN summary")
		return
	}

	summaryByID := make(map[string]model.SiteToSiteVpnSummary, len(summaries))
	for _, summary := range summaries {
		summaryByID[summary.ID] = summary
	}

	s2sStats, err := client.GetSiteToSiteVpnStats()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get site-to-site VPN stats")
		return
	}
	c.collectSiteToSiteVpnMetrics(ch, site, s2sStats, summaryByID, seenPacketSeries)

	for _, summary := range summaries {
		peerStats, err := client.GetSiteToSiteVpnPeerStats(summary.ID)
		if err != nil {
			log.Error().Err(err).Str("vpn_id", summary.ID).Msg("Failed to get site-to-site VPN peer stats")
			continue
		}
		c.collectSiteToSiteVpnPeerMetrics(ch, site, summary, peerStats)
	}
}

// collectVpnTunnelMetrics emits metrics for the VPN tunnel metrics.
func (c *vpnStatsCollector) collectVpnTunnelMetrics(ch chan<- prometheus.Metric, site string, vpn []model.VpnStats) map[string]struct{} {
	seenPacketSeries := make(map[string]struct{}, len(vpn))

	for _, item := range vpn {
		labels := []string{item.Name, item.InterfaceName, item.GetVpnMode(), item.GetVpnType(), item.LocalIp, item.RemoteIp, site, c.client.SiteId}
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUptime, prometheus.GaugeValue, float64(item.GetUptime()), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnDownPackets, prometheus.GaugeValue, float64(item.DownPkts), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnDownBytes, prometheus.GaugeValue, float64(item.DownBytes), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUpPackets, prometheus.GaugeValue, float64(item.UpPkts), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUpBytes, prometheus.GaugeValue, float64(item.UpBytes), labels...)
		seenPacketSeries[vpnPacketSeriesKey(item.Name, item.InterfaceName, item.GetVpnMode(), item.GetVpnType(), item.LocalIp, item.RemoteIp)] = struct{}{}
	}

	return seenPacketSeries
}

// collectSiteToSiteVpnMetrics emits metrics for the site to site VPN metrics.
func (c *vpnStatsCollector) collectSiteToSiteVpnMetrics(ch chan<- prometheus.Metric, site string, stats []model.SiteToSiteVpnStats, summaryByID map[string]model.SiteToSiteVpnSummary, seenPacketSeries map[string]struct{}) {
	for _, item := range stats {
		summary, ok := summaryByID[item.VpnID]
		name := item.Name
		vpnType := item.GetVpnType()
		siteVpnType := ""
		if ok {
			name = firstNonEmpty(summary.Name, name)
			vpnType = firstNonEmpty(summary.GetVpnType(), vpnType)
			siteVpnType = summary.GetSiteVpnType()
		}

		labels := []string{
			item.VpnID,
			firstNonEmpty(item.ID, item.VpnID),
			name,
			vpnType,
			siteVpnType,
			item.InterfaceName,
			item.Direction,
			item.LocalIP,
			item.RemoteIP,
			item.LocalPeerIP,
			item.RemotePeerIP,
			site,
			c.client.SiteId,
		}

		vpnPacketLabels := []string{name, item.InterfaceName, item.GetVpnMode(), vpnType, item.LocalIP, item.RemoteIP, site, c.client.SiteId}
		packetSeriesKey := vpnPacketSeriesKey(name, item.InterfaceName, item.GetVpnMode(), vpnType, item.LocalIP, item.RemoteIP)
		if _, exists := seenPacketSeries[packetSeriesKey]; !exists {
			ch <- prometheus.MustNewConstMetric(c.omadaVpnDownPackets, prometheus.GaugeValue, float64(item.DownPkts), vpnPacketLabels...)
			ch <- prometheus.MustNewConstMetric(c.omadaVpnUpPackets, prometheus.GaugeValue, float64(item.UpPkts), vpnPacketLabels...)
			seenPacketSeries[packetSeriesKey] = struct{}{}
		}

		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnConnectedPeers, prometheus.GaugeValue, float64(item.ConnectedNum), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnDisconnectedPeers, prometheus.GaugeValue, float64(item.DisconnectedNum), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnTotalPeers, prometheus.GaugeValue, float64(item.TotalRemoteNum), labels...)
	}
}

// collectSiteToSiteVpnPeerMetrics emits metrics for the site to site VPN peer metrics.
func (c *vpnStatsCollector) collectSiteToSiteVpnPeerMetrics(ch chan<- prometheus.Metric, site string, summary model.SiteToSiteVpnSummary, peerStats []model.SiteToSiteVpnPeerStats) {
	for _, item := range peerStats {
		labels := []string{
			summary.ID,
			summary.Name,
			item.ID,
			item.Name,
			summary.GetVpnType(),
			summary.GetSiteVpnType(),
			item.LocalIP,
			item.RemoteIP,
			strconv.Itoa(int(item.Port)),
			site,
			c.client.SiteId,
		}

		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerStatus, prometheus.GaugeValue, item.GetStatus(), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerDownBytes, prometheus.GaugeValue, float64(item.DownBytes), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerUpBytes, prometheus.GaugeValue, float64(item.UpBytes), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerDownPackets, prometheus.GaugeValue, float64(item.DownPkts), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerUpPackets, prometheus.GaugeValue, float64(item.UpPkts), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerLoginTimestamp, prometheus.GaugeValue, normalizeUnixTimestampSeconds(item.LoginTime), labels...)
	}
}

// NewVpnStatsCollector builds the Prometheus descriptors used to export VPN
// tunnel, site-to-site, and peer statistics.
func NewVpnStatsCollector(apiClient *api.Client) *vpnStatsCollector {
	labels := []string{"name", "interface_name", "vpn_mode", "vpn_type", "local_ip", "remote_ip", "site", "site_id"}
	siteToSiteLabels := []string{"vpn_id", "tunnel_id", "name", "vpn_type", "site_vpn_type", "interface_name", "direction", "local_ip", "remote_ip", "local_peer_ip", "remote_peer_ip", "site", "site_id"}
	siteToSitePeerLabels := []string{"vpn_id", "name", "peer_id", "peer_name", "vpn_type", "site_vpn_type", "local_ip", "remote_ip", "port", "site", "site_id"}

	return &vpnStatsCollector{
		omadaVpnUptime: prometheus.NewDesc("omada_vpn_uptime",
			"The current uptime of the VPN",
			labels,
			nil,
		),
		omadaVpnDownPackets: prometheus.NewDesc("omada_vpn_down_packets",
			"VPN downlink traffic in packets",
			labels,
			nil,
		),
		omadaVpnDownBytes: prometheus.NewDesc("omada_vpn_down_bytes",
			"VPN downlink traffic in bytes",
			labels,
			nil,
		),
		omadaVpnUpPackets: prometheus.NewDesc("omada_vpn_up_packets",
			"VPN uplink traffic in packets",
			labels,
			nil,
		),
		omadaVpnUpBytes: prometheus.NewDesc("omada_vpn_up_bytes",
			"VPN uplink traffic in bytes",
			labels,
			nil,
		),
		omadaSiteToSiteVpnConnectedPeers: prometheus.NewDesc("omada_site_to_site_vpn_connected_peers",
			"Number of connected site-to-site VPN peers",
			siteToSiteLabels,
			nil,
		),
		omadaSiteToSiteVpnDisconnectedPeers: prometheus.NewDesc("omada_site_to_site_vpn_disconnected_peers",
			"Number of disconnected site-to-site VPN peers",
			siteToSiteLabels,
			nil,
		),
		omadaSiteToSiteVpnTotalPeers: prometheus.NewDesc("omada_site_to_site_vpn_total_peers",
			"Total number of site-to-site VPN peers",
			siteToSiteLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerStatus: prometheus.NewDesc("omada_site_to_site_vpn_peer_status",
			"The current runtime status of the site-to-site VPN peer",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerDownBytes: prometheus.NewDesc("omada_site_to_site_vpn_peer_down_bytes",
			"Site-to-site VPN peer downlink traffic in bytes",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerUpBytes: prometheus.NewDesc("omada_site_to_site_vpn_peer_up_bytes",
			"Site-to-site VPN peer uplink traffic in bytes",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerDownPackets: prometheus.NewDesc("omada_site_to_site_vpn_peer_down_packets",
			"Site-to-site VPN peer downlink traffic in packets",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerUpPackets: prometheus.NewDesc("omada_site_to_site_vpn_peer_up_packets",
			"Site-to-site VPN peer uplink traffic in packets",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerLoginTimestamp: prometheus.NewDesc("omada_site_to_site_vpn_peer_login_timestamp",
			"Unix login timestamp of the site-to-site VPN peer in seconds",
			siteToSitePeerLabels,
			nil,
		),
		client: &openapi.Client{
			Client: apiClient,
		},
	}
}

// firstNonEmpty returns the first non-empty string in the provided values.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// normalizeUnixTimestampSeconds normalizes a Unix timestamp value to seconds.
func normalizeUnixTimestampSeconds(value int64) float64 {
	switch {
	case value <= 0:
		return 0
	case value >= 1_000_000_000_000:
		return float64(value) / 1000
	default:
		return float64(value)
	}
}

// vpnPacketSeriesKey builds a unique key for a VPN packet metric series.
func vpnPacketSeriesKey(name, interfaceName, vpnMode, vpnType, localIP, remoteIP string) string {
	return firstNonEmpty(name) + "|" + firstNonEmpty(interfaceName) + "|" + firstNonEmpty(vpnMode) + "|" + firstNonEmpty(vpnType) + "|" + firstNonEmpty(localIP) + "|" + firstNonEmpty(remoteIP)
}
