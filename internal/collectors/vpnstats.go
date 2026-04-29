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
	omadaSiteToSiteVpnDownBytes          *prometheus.Desc
	omadaSiteToSiteVpnUpBytes            *prometheus.Desc
	omadaSiteToSiteVpnTotalPeers         *prometheus.Desc
	omadaSiteToSiteVpnPeerDownBytes      *prometheus.Desc
	omadaSiteToSiteVpnPeerUpBytes        *prometheus.Desc
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
	ch <- c.omadaSiteToSiteVpnDownBytes
	ch <- c.omadaSiteToSiteVpnUpBytes
	ch <- c.omadaSiteToSiteVpnTotalPeers
	ch <- c.omadaSiteToSiteVpnPeerDownBytes
	ch <- c.omadaSiteToSiteVpnPeerUpBytes
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

	peerStatsByVpnID := make(map[string][]model.SiteToSiteVpnPeerStats, len(summaryByID))
	for _, summary := range summaries {
		peerStats, err := client.GetSiteToSiteVpnPeerStats(summary.ID)
		if err != nil {
			log.Error().Err(err).Str("vpn_id", summary.ID).Msg("Failed to get site-to-site VPN peer stats")
			continue
		}
		peerStatsByVpnID[summary.ID] = append(peerStatsByVpnID[summary.ID], peerStats...)
	}

	c.collectSiteToSiteVpnMetrics(ch, site, s2sStats, summaryByID, peerStatsByVpnID, seenPacketSeries)
	c.collectSiteToSiteVpnPeerMetrics(ch, site, summaries, peerStatsByVpnID)
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
func (c *vpnStatsCollector) collectSiteToSiteVpnMetrics(ch chan<- prometheus.Metric, site string, stats []model.SiteToSiteVpnStats, summaryByID map[string]model.SiteToSiteVpnSummary, peerStatsByVpnID map[string][]model.SiteToSiteVpnPeerStats, seenPacketSeries map[string]struct{}) {
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
		if shouldEmitVpnPacketSeries(item) {
			if _, exists := seenPacketSeries[packetSeriesKey]; !exists {
				ch <- prometheus.MustNewConstMetric(c.omadaVpnDownPackets, prometheus.GaugeValue, float64(item.DownPkts), vpnPacketLabels...)
				ch <- prometheus.MustNewConstMetric(c.omadaVpnUpPackets, prometheus.GaugeValue, float64(item.UpPkts), vpnPacketLabels...)
				seenPacketSeries[packetSeriesKey] = struct{}{}
			}
		}

		downBytes, upBytes := aggregateSiteToSitePeerBytes(peerStatsByVpnID[item.VpnID])
		if downBytes == 0 && upBytes == 0 {
			downBytes = item.DownBytes
			upBytes = item.UpBytes
		}

		siteToSiteTrafficLabels := []string{
			item.VpnID,
			name,
			vpnType,
			siteVpnType,
			site,
			c.client.SiteId,
		}
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnDownBytes, prometheus.GaugeValue, float64(downBytes), siteToSiteTrafficLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnUpBytes, prometheus.GaugeValue, float64(upBytes), siteToSiteTrafficLabels...)

		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnTotalPeers, prometheus.GaugeValue, float64(item.TotalRemoteNum), labels...)
	}
}

// collectSiteToSiteVpnPeerMetrics emits metrics for the site to site VPN peer metrics.
func (c *vpnStatsCollector) collectSiteToSiteVpnPeerMetrics(ch chan<- prometheus.Metric, site string, summaries []model.SiteToSiteVpnSummary, peerStatsByVpnID map[string][]model.SiteToSiteVpnPeerStats) {
	for _, summary := range summaries {
		for _, item := range peerStatsByVpnID[summary.ID] {
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

			ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerDownBytes, prometheus.GaugeValue, float64(item.DownBytes), labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerUpBytes, prometheus.GaugeValue, float64(item.UpBytes), labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerLoginTimestamp, prometheus.GaugeValue, normalizeUnixTimestampSeconds(item.LoginTime), labels...)
		}
	}
}

// NewVpnStatsCollector builds the Prometheus descriptors used to export VPN
// tunnel, site-to-site, and peer statistics.
func NewVpnStatsCollector(apiClient *api.Client) *vpnStatsCollector {
	labels := []string{"name", "interface_name", "vpn_mode", "vpn_type", "local_ip", "remote_ip", "site", "site_id"}
	siteToSiteTrafficLabels := []string{"vpn_id", "name", "vpn_type", "site_vpn_type", "site", "site_id"}
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
		omadaSiteToSiteVpnDownBytes: prometheus.NewDesc("omada_site_to_site_vpn_down_bytes",
			"Site-to-site VPN downlink traffic in bytes aggregated across peers when needed",
			siteToSiteTrafficLabels,
			nil,
		),
		omadaSiteToSiteVpnUpBytes: prometheus.NewDesc("omada_site_to_site_vpn_up_bytes",
			"Site-to-site VPN uplink traffic in bytes aggregated across peers when needed",
			siteToSiteTrafficLabels,
			nil,
		),
		omadaSiteToSiteVpnTotalPeers: prometheus.NewDesc("omada_site_to_site_vpn_total_peers",
			"Total number of site-to-site VPN peers",
			siteToSiteLabels,
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

// shouldEmitVpnPacketSeries reports whether site-to-site tunnel stats provide enough context to expose packet metrics.
func shouldEmitVpnPacketSeries(item model.SiteToSiteVpnStats) bool {
	return item.InterfaceName != "" || item.LocalIP != "" || item.RemoteIP != "" || item.DownPkts != 0 || item.UpPkts != 0
}

// aggregateSiteToSitePeerBytes sums peer byte counters for a site-to-site VPN.
func aggregateSiteToSitePeerBytes(peerStats []model.SiteToSiteVpnPeerStats) (int64, int64) {
	var downBytes int64
	var upBytes int64

	for _, item := range peerStats {
		downBytes += item.DownBytes
		upBytes += item.UpBytes
	}

	return downBytes, upBytes
}
