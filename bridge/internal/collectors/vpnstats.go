package collector

import (
	"strconv"
	"strings"

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
	omadaSiteToSiteVpnPeerStatus         *prometheus.Desc
	omadaSiteToSiteVpnPeerDownPackets    *prometheus.Desc
	omadaSiteToSiteVpnPeerDownBytes      *prometheus.Desc
	omadaSiteToSiteVpnPeerUpPackets      *prometheus.Desc
	omadaSiteToSiteVpnPeerUpBytes        *prometheus.Desc
	omadaSiteToSiteVpnPeerLoginTimestamp *prometheus.Desc
	client                               *openapi.Client
}

type vpnTunnelSeries struct {
	labels      []string
	uptime      int64
	downPackets int64
	downBytes   int64
	upPackets   int64
	upBytes     int64
}

type siteToSiteTrafficValue struct {
	downBytes int64
	upBytes   int64
}

type siteToSitePeerTraffic struct {
	value   siteToSiteTrafficValue
	hasDown bool
	hasUp   bool
}

type siteToSitePacketValue struct {
	downPackets int64
	upPackets   int64
}

type siteToSitePacketSource struct {
	labels []string
	value  siteToSitePacketValue
}

type siteToSitePacketSeries struct {
	labels []string
	value  siteToSitePacketValue
}

type siteToSiteTrafficSeries struct {
	labels []string
	value  siteToSiteTrafficValue
}

type siteToSiteTrafficSource struct {
	vpnID  string
	labels []string
	value  siteToSiteTrafficValue
}

type siteToSiteGaugeSeries struct {
	labels []string
	value  int64
}

type siteToSitePeerSeries struct {
	labels         []string
	status         float64
	hasStatus      bool
	downPackets    int64
	hasDownPackets bool
	downBytes      int64
	upPackets      int64
	hasUpPackets   bool
	upBytes        int64
	loginTimestamp float64
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
	ch <- c.omadaSiteToSiteVpnPeerStatus
	ch <- c.omadaSiteToSiteVpnPeerDownPackets
	ch <- c.omadaSiteToSiteVpnPeerDownBytes
	ch <- c.omadaSiteToSiteVpnPeerUpPackets
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
		_, tunnelSiteID := client.ContextIDs()
		seenPacketSeries = c.collectVpnTunnelMetrics(ch, site, tunnelSiteID, vpn)
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
	peerStatsCompleteByVpnID := make(map[string]bool, len(summaryByID))
	for vpnID, tunnelIDs := range buildSiteToSiteTunnelIDsByVpnID(s2sStats) {
		complete := true
		for _, tunnelID := range tunnelIDs {
			peerStats, err := client.GetSiteToSiteVpnPeerStats(tunnelID)
			if err != nil {
				complete = false
				log.Error().Err(err).Str("vpn_id", vpnID).Str("tunnel_id", tunnelID).Msg("Failed to get site-to-site VPN peer stats")
				continue
			}
			peerStatsByVpnID[vpnID] = append(peerStatsByVpnID[vpnID], peerStats...)
		}
		peerStatsCompleteByVpnID[vpnID] = complete
	}

	_, siteToSiteSiteID := client.ContextIDs()
	c.collectSiteToSiteVpnMetrics(ch, site, siteToSiteSiteID, s2sStats, summaryByID, peerStatsByVpnID, peerStatsCompleteByVpnID, seenPacketSeries)
	_, peerSiteID := client.ContextIDs()
	c.collectSiteToSiteVpnPeerMetrics(ch, site, peerSiteID, summaries, peerStatsByVpnID)
}

// collectVpnTunnelMetrics emits metrics for the VPN tunnel metrics.
func (c *vpnStatsCollector) collectVpnTunnelMetrics(ch chan<- prometheus.Metric, site, siteID string, vpn []model.VpnStats) map[string]struct{} {
	seenPacketSeries := make(map[string]struct{}, len(vpn))
	sourceByKey := make(map[string]*vpnTunnelSeries, len(vpn))
	sourceOrder := make([]string, 0, len(vpn))

	for _, item := range vpn {
		labels := []string{item.Name, item.InterfaceName, item.GetVpnMode(), item.GetVpnType(), item.LocalIp, item.RemoteIp, site, siteID}
		labels = append(labels, vpnDetailValuesWithoutLocalIP(item.DetailLabels())...)
		logicalKey := vpnPacketLogicalSeriesKey(item.Name, item.InterfaceName, item.GetVpnMode(), item.GetVpnType(), item.LocalIp, item.RemoteIp, site, siteID)
		sourceKey := vpnTunnelSourceKey(item, site, siteID)
		source, exists := sourceByKey[sourceKey]
		if !exists {
			source = &vpnTunnelSeries{labels: labels}
			sourceByKey[sourceKey] = source
			sourceOrder = append(sourceOrder, sourceKey)
		} else if preferMetricLabels(labels, source.labels) {
			source.labels = labels
		}
		source.uptime = max(source.uptime, item.GetUptime())
		source.downPackets = max(source.downPackets, item.DownPkts)
		source.downBytes = max(source.downBytes, item.DownBytes)
		source.upPackets = max(source.upPackets, item.UpPkts)
		source.upBytes = max(source.upBytes, item.UpBytes)
		seenPacketSeries[logicalKey] = struct{}{}
	}

	seriesByKey := make(map[string]*vpnTunnelSeries, len(sourceByKey))
	seriesOrder := make([]string, 0, len(sourceByKey))
	for _, sourceKey := range sourceOrder {
		source := sourceByKey[sourceKey]
		seriesKey := prometheusLabelValuesKey(source.labels)
		series, exists := seriesByKey[seriesKey]
		if !exists {
			series = &vpnTunnelSeries{labels: source.labels}
			seriesByKey[seriesKey] = series
			seriesOrder = append(seriesOrder, seriesKey)
		}
		series.uptime = max(series.uptime, source.uptime)
		series.downPackets += source.downPackets
		series.downBytes += source.downBytes
		series.upPackets += source.upPackets
		series.upBytes += source.upBytes
	}

	for _, key := range seriesOrder {
		series := seriesByKey[key]
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUptime, prometheus.GaugeValue, float64(series.uptime), series.labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnDownPackets, prometheus.GaugeValue, float64(series.downPackets), series.labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnDownBytes, prometheus.GaugeValue, float64(series.downBytes), series.labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUpPackets, prometheus.GaugeValue, float64(series.upPackets), series.labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUpBytes, prometheus.GaugeValue, float64(series.upBytes), series.labels...)
	}

	return seenPacketSeries
}

// collectSiteToSiteVpnMetrics emits metrics for the site to site VPN metrics.
func (c *vpnStatsCollector) collectSiteToSiteVpnMetrics(ch chan<- prometheus.Metric, site, siteID string, stats []model.SiteToSiteVpnStats, summaryByID map[string]model.SiteToSiteVpnSummary, peerStatsByVpnID map[string][]model.SiteToSiteVpnPeerStats, peerStatsCompleteByVpnID map[string]bool, seenPacketSeries map[string]struct{}) {
	packetBySourceKey := make(map[string]*siteToSitePacketSource, len(stats))
	packetSourceOrder := make([]string, 0, len(stats))
	packetSourcesCoveredByTunnelStats := make(map[string]struct{})
	rawTrafficBySourceKey := make(map[string]*siteToSiteTrafficSource, len(stats))
	rawTrafficSourceOrder := make([]string, 0, len(stats))
	peerTrafficByVpnID := make(map[string]siteToSitePeerTraffic)
	peerTrafficLabelsByVpnID := make(map[string][]string)
	totalPeersBySourceKey := make(map[string]*siteToSiteGaugeSeries, len(stats))
	totalPeerSourceOrder := make([]string, 0, len(stats))
	peerTotalsByVpnID := make(map[string]siteToSiteTrafficValue, len(peerStatsByVpnID))
	for vpnID, peerStats := range peerStatsByVpnID {
		downBytes, upBytes := aggregateSiteToSitePeerBytes(peerStats, summaryByID[vpnID])
		peerTotalsByVpnID[vpnID] = siteToSiteTrafficValue{downBytes: downBytes, upBytes: upBytes}
	}

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
		detailLabels := item.DetailLabels(summary)

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
			siteID,
		}
		labels = append(labels, vpnDetailValuesWithoutLocalIP(detailLabels)...)

		sourceKey := siteToSiteVpnStatsSourceKey(item)
		vpnPacketLabels := []string{name, item.InterfaceName, item.GetVpnMode(), vpnType, item.LocalIP, item.RemoteIP, site, siteID}
		vpnPacketLabels = append(vpnPacketLabels, vpnDetailValuesWithoutLocalIP(detailLabels)...)
		if shouldEmitVpnPacketSeries(item) {
			logicalKey := vpnPacketLogicalSeriesKey(name, item.InterfaceName, item.GetVpnMode(), vpnType, item.LocalIP, item.RemoteIP, site, siteID)
			if _, covered := seenPacketSeries[logicalKey]; covered {
				packetSourcesCoveredByTunnelStats[sourceKey] = struct{}{}
			} else {
				packetSource, exists := packetBySourceKey[sourceKey]
				if !exists {
					packetSource = &siteToSitePacketSource{labels: vpnPacketLabels}
					packetBySourceKey[sourceKey] = packetSource
					packetSourceOrder = append(packetSourceOrder, sourceKey)
				} else if preferMetricLabels(vpnPacketLabels, packetSource.labels) {
					packetSource.labels = vpnPacketLabels
				}
				packetSource.value.downPackets = max(packetSource.value.downPackets, item.DownPkts)
				packetSource.value.upPackets = max(packetSource.value.upPackets, item.UpPkts)
			}
		}

		peerTotals := peerTotalsByVpnID[item.VpnID]

		siteToSiteTrafficLabels := []string{
			item.VpnID,
			name,
			vpnType,
			siteVpnType,
			site,
			siteID,
		}
		siteToSiteTrafficLabels = append(siteToSiteTrafficLabels, detailLabels.Values()...)

		source, exists := rawTrafficBySourceKey[sourceKey]
		if !exists {
			source = &siteToSiteTrafficSource{
				vpnID:  item.VpnID,
				labels: siteToSiteTrafficLabels,
			}
			rawTrafficBySourceKey[sourceKey] = source
			rawTrafficSourceOrder = append(rawTrafficSourceOrder, sourceKey)
		} else if preferMetricLabels(siteToSiteTrafficLabels, source.labels) {
			source.labels = siteToSiteTrafficLabels
		}
		source.value.downBytes = max(source.value.downBytes, item.DownBytes)
		source.value.upBytes = max(source.value.upBytes, item.UpBytes)

		if item.VpnID != "" && peerStatsCompleteByVpnID[item.VpnID] && (peerTotals.downBytes != 0 || peerTotals.upBytes != 0) {
			peerTraffic := peerTrafficByVpnID[item.VpnID]
			if peerTotals.downBytes != 0 {
				peerTraffic.hasDown = true
				peerTraffic.value.downBytes = max(peerTraffic.value.downBytes, peerTotals.downBytes)
			}
			if peerTotals.upBytes != 0 {
				peerTraffic.hasUp = true
				peerTraffic.value.upBytes = max(peerTraffic.value.upBytes, peerTotals.upBytes)
			}
			peerTrafficByVpnID[item.VpnID] = peerTraffic

			currentLabels, hasCurrentLabels := peerTrafficLabelsByVpnID[item.VpnID]
			if !hasCurrentLabels || preferMetricLabels(siteToSiteTrafficLabels, currentLabels) {
				peerTrafficLabelsByVpnID[item.VpnID] = siteToSiteTrafficLabels
			}
		}

		totalPeersSeries, exists := totalPeersBySourceKey[sourceKey]
		if !exists {
			totalPeersSeries = &siteToSiteGaugeSeries{labels: labels}
			totalPeersBySourceKey[sourceKey] = totalPeersSeries
			totalPeerSourceOrder = append(totalPeerSourceOrder, sourceKey)
		} else if preferMetricLabels(labels, totalPeersSeries.labels) {
			totalPeersSeries.labels = labels
		}
		totalPeersSeries.value = max(totalPeersSeries.value, item.TotalRemoteNum)
	}

	packetByKey := make(map[string]*siteToSitePacketSeries, len(packetBySourceKey))
	packetOrder := make([]string, 0, len(packetBySourceKey))
	for _, sourceKey := range packetSourceOrder {
		if _, covered := packetSourcesCoveredByTunnelStats[sourceKey]; covered {
			continue
		}
		source := packetBySourceKey[sourceKey]
		packetKey := prometheusLabelValuesKey(source.labels)
		series, exists := packetByKey[packetKey]
		if !exists {
			series = &siteToSitePacketSeries{labels: source.labels}
			packetByKey[packetKey] = series
			packetOrder = append(packetOrder, packetKey)
		}
		series.value.downPackets += source.value.downPackets
		series.value.upPackets += source.value.upPackets
	}
	for _, key := range packetOrder {
		series := packetByKey[key]
		ch <- prometheus.MustNewConstMetric(c.omadaVpnDownPackets, prometheus.GaugeValue, float64(series.value.downPackets), series.labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaVpnUpPackets, prometheus.GaugeValue, float64(series.value.upPackets), series.labels...)
	}

	trafficByKey := make(map[string]*siteToSiteTrafficSeries, len(rawTrafficBySourceKey))
	trafficOrder := make([]string, 0, len(rawTrafficBySourceKey))
	emittedPeerTraffic := make(map[string]struct{}, len(peerTrafficByVpnID))
	rawTrafficByVpnID := make(map[string]siteToSiteTrafficValue, len(peerTrafficByVpnID))
	for _, sourceKey := range rawTrafficSourceOrder {
		source := rawTrafficBySourceKey[sourceKey]
		if source.vpnID == "" {
			continue
		}
		value := rawTrafficByVpnID[source.vpnID]
		value.downBytes += source.value.downBytes
		value.upBytes += source.value.upBytes
		rawTrafficByVpnID[source.vpnID] = value
	}
	for _, sourceKey := range rawTrafficSourceOrder {
		source := rawTrafficBySourceKey[sourceKey]
		labels := source.labels
		value := source.value
		if peerTraffic, hasPeerTraffic := peerTrafficByVpnID[source.vpnID]; hasPeerTraffic {
			if _, emitted := emittedPeerTraffic[source.vpnID]; emitted {
				continue
			}
			emittedPeerTraffic[source.vpnID] = struct{}{}
			labels = peerTrafficLabelsByVpnID[source.vpnID]
			value = rawTrafficByVpnID[source.vpnID]
			if peerTraffic.hasDown {
				value.downBytes = peerTraffic.value.downBytes
			}
			if peerTraffic.hasUp {
				value.upBytes = peerTraffic.value.upBytes
			}
		}

		trafficKey := prometheusLabelValuesKey(labels)
		trafficSeries, exists := trafficByKey[trafficKey]
		if !exists {
			trafficSeries = &siteToSiteTrafficSeries{labels: labels}
			trafficByKey[trafficKey] = trafficSeries
			trafficOrder = append(trafficOrder, trafficKey)
		}
		trafficSeries.value.downBytes += value.downBytes
		trafficSeries.value.upBytes += value.upBytes
	}

	for _, key := range trafficOrder {
		series := trafficByKey[key]
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnDownBytes, prometheus.GaugeValue, float64(series.value.downBytes), series.labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnUpBytes, prometheus.GaugeValue, float64(series.value.upBytes), series.labels...)
	}
	totalPeersByKey := make(map[string]*siteToSiteGaugeSeries, len(totalPeersBySourceKey))
	totalPeersOrder := make([]string, 0, len(totalPeersBySourceKey))
	for _, sourceKey := range totalPeerSourceOrder {
		source := totalPeersBySourceKey[sourceKey]
		key := prometheusLabelValuesKey(source.labels)
		series, exists := totalPeersByKey[key]
		if !exists {
			series = &siteToSiteGaugeSeries{labels: source.labels}
			totalPeersByKey[key] = series
			totalPeersOrder = append(totalPeersOrder, key)
		}
		series.value = max(series.value, source.value)
	}
	for _, key := range totalPeersOrder {
		series := totalPeersByKey[key]
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnTotalPeers, prometheus.GaugeValue, float64(series.value), series.labels...)
	}
}

// collectSiteToSiteVpnPeerMetrics emits metrics for the site to site VPN peer metrics.
func (c *vpnStatsCollector) collectSiteToSiteVpnPeerMetrics(ch chan<- prometheus.Metric, site, siteID string, summaries []model.SiteToSiteVpnSummary, peerStatsByVpnID map[string][]model.SiteToSiteVpnPeerStats) {
	sourceByKey := make(map[string]*siteToSitePeerSeries)
	sourceOrder := make([]string, 0)

	for _, summary := range summaries {
		for _, item := range peerStatsByVpnID[summary.ID] {
			detailLabels := item.DetailLabels(summary)
			labels := []string{
				summary.ID,
				summary.Name,
				siteToSitePeerID(item),
				item.Name,
				summary.GetVpnType(),
				summary.GetSiteVpnType(),
				item.LocalIP,
				item.RemoteIP,
				strconv.Itoa(int(item.Port)),
				site,
				siteID,
			}
			labels = append(labels, vpnDetailValuesWithoutLocalIP(detailLabels)...)
			seriesKey := prometheusLabelValuesKey([]string{
				siteToSiteVpnSummarySourceKey(summary),
				siteToSiteVpnPeerSourceKey(item, summary),
			})
			series, exists := sourceByKey[seriesKey]
			if !exists {
				series = &siteToSitePeerSeries{labels: labels}
				sourceByKey[seriesKey] = series
				sourceOrder = append(sourceOrder, seriesKey)
			} else if preferMetricLabels(labels, series.labels) {
				series.labels = labels
			}

			if status, ok := item.GetStatus(); ok {
				series.hasStatus = true
				series.status = max(series.status, status)
			}
			if item.DownPkts != nil {
				series.hasDownPackets = true
				series.downPackets = max(series.downPackets, *item.DownPkts)
			}
			series.downBytes = max(series.downBytes, item.DownBytes)
			if item.UpPkts != nil {
				series.hasUpPackets = true
				series.upPackets = max(series.upPackets, *item.UpPkts)
			}
			series.upBytes = max(series.upBytes, item.UpBytes)
			series.loginTimestamp = max(series.loginTimestamp, normalizeUnixTimestampSeconds(item.LoginTime))
		}
	}

	seriesByKey := make(map[string]*siteToSitePeerSeries, len(sourceByKey))
	seriesOrder := make([]string, 0, len(sourceByKey))
	for _, sourceKey := range sourceOrder {
		source := sourceByKey[sourceKey]
		seriesKey := prometheusLabelValuesKey(source.labels)
		series, exists := seriesByKey[seriesKey]
		if !exists {
			copiedSeries := *source
			series = &copiedSeries
			seriesByKey[seriesKey] = series
			seriesOrder = append(seriesOrder, seriesKey)
			continue
		}
		mergeSiteToSitePeerSeries(series, source)
	}

	for _, key := range seriesOrder {
		series := seriesByKey[key]
		if series.hasStatus {
			ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerStatus, prometheus.GaugeValue, series.status, series.labels...)
		}
		if series.hasDownPackets {
			ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerDownPackets, prometheus.GaugeValue, float64(series.downPackets), series.labels...)
		}
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerDownBytes, prometheus.GaugeValue, float64(series.downBytes), series.labels...)
		if series.hasUpPackets {
			ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerUpPackets, prometheus.GaugeValue, float64(series.upPackets), series.labels...)
		}
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerUpBytes, prometheus.GaugeValue, float64(series.upBytes), series.labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaSiteToSiteVpnPeerLoginTimestamp, prometheus.GaugeValue, series.loginTimestamp, series.labels...)
	}
}

// NewVpnStatsCollector builds the Prometheus descriptors used to export VPN
// tunnel, site-to-site, and peer statistics.
func NewVpnStatsCollector(apiClient *api.Client) *vpnStatsCollector {
	labels := []string{"name", "interface_name", "vpn_mode", "vpn_type", "local_ip", "remote_ip", "site", "site_id"}
	siteToSiteTrafficLabels := []string{"vpn_id", "name", "vpn_type", "site_vpn_type", "site", "site_id"}
	siteToSiteLabels := []string{"vpn_id", "tunnel_id", "name", "vpn_type", "site_vpn_type", "interface_name", "direction", "local_ip", "remote_ip", "local_peer_ip", "remote_peer_ip", "site", "site_id"}
	siteToSitePeerLabels := []string{"vpn_id", "name", "peer_id", "peer_name", "vpn_type", "site_vpn_type", "local_ip", "remote_ip", "port", "site", "site_id"}
	detailLabels := model.VPNDetailLabelNames()
	labels = append(labels, detailLabels[1:]...)
	siteToSiteTrafficLabels = append(siteToSiteTrafficLabels, detailLabels...)
	siteToSiteLabels = append(siteToSiteLabels, detailLabels[1:]...)
	siteToSitePeerLabels = append(siteToSitePeerLabels, detailLabels[1:]...)

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
		omadaSiteToSiteVpnPeerStatus: prometheus.NewDesc("omada_site_to_site_vpn_peer_status",
			"Site-to-site VPN peer online status",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerDownPackets: prometheus.NewDesc("omada_site_to_site_vpn_peer_down_packets",
			"Site-to-site VPN peer downlink traffic in packets",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerDownBytes: prometheus.NewDesc("omada_site_to_site_vpn_peer_down_bytes",
			"Site-to-site VPN peer downlink traffic in bytes",
			siteToSitePeerLabels,
			nil,
		),
		omadaSiteToSiteVpnPeerUpPackets: prometheus.NewDesc("omada_site_to_site_vpn_peer_up_packets",
			"Site-to-site VPN peer uplink traffic in packets",
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

func vpnDetailValuesWithoutLocalIP(labels model.VPNDetailLabels) []string {
	return labels.Values()[1:]
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

// prometheusLabelValuesKey creates a collision-safe identity for a metric's
// ordered variable-label values.
func prometheusLabelValuesKey(values []string) string {
	var key strings.Builder
	for _, value := range values {
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	return key.String()
}

// vpnPacketLogicalSeriesKey matches packet statistics for the same logical VPN
// across the general tunnel and site-to-site endpoints. Optional enrichment
// labels are excluded because one endpoint can omit them.
func vpnPacketLogicalSeriesKey(name, interfaceName, vpnMode, vpnType, localIP, remoteIP, site, siteID string) string {
	return prometheusLabelValuesKey([]string{name, interfaceName, vpnMode, vpnType, localIP, remoteIP, site, siteID})
}

// vpnTunnelSourceKey keeps distinct sessions under one VPN configuration while
// allowing optional endpoint and network enrichment to change between snapshots.
func vpnTunnelSourceKey(item model.VpnStats, site, siteID string) string {
	values := []string{
		item.VpnID,
		item.InterfaceName,
		item.GetVpnMode(),
		item.GetVpnType(),
		item.LocalIp,
		item.RemoteIp,
		site,
		siteID,
	}
	if item.VpnID == "" {
		values = append(values, item.Name)
	}
	return prometheusLabelValuesKey(values)
}

// preferMetricLabels selects a deterministic representative when a VPN-level
// peer aggregate could otherwise be repeated across multiple detail-label sets.
func preferMetricLabels(candidate, current []string) bool {
	candidatePopulated := 0
	for _, value := range candidate {
		if strings.TrimSpace(value) != "" {
			candidatePopulated++
		}
	}
	currentPopulated := 0
	for _, value := range current {
		if strings.TrimSpace(value) != "" {
			currentPopulated++
		}
	}
	if candidatePopulated != currentPopulated {
		return candidatePopulated > currentPopulated
	}
	return prometheusLabelValuesKey(candidate) < prometheusLabelValuesKey(current)
}

// mergeSiteToSitePeerSeries coalesces two sources that project to the same
// final Prometheus label set.
func mergeSiteToSitePeerSeries(target, source *siteToSitePeerSeries) {
	if source.hasStatus {
		target.hasStatus = true
		target.status = max(target.status, source.status)
	}
	if source.hasDownPackets {
		target.hasDownPackets = true
		target.downPackets = max(target.downPackets, source.downPackets)
	}
	target.downBytes = max(target.downBytes, source.downBytes)
	if source.hasUpPackets {
		target.hasUpPackets = true
		target.upPackets = max(target.upPackets, source.upPackets)
	}
	target.upBytes = max(target.upBytes, source.upBytes)
	target.loginTimestamp = max(target.loginTimestamp, source.loginTimestamp)
}

// siteToSiteVpnStatsSourceKey identifies one logical tunnel statistics row.
// Counters are deliberately excluded so duplicate snapshots can be coalesced
// by taking the highest value without double-counting them.
func siteToSiteVpnStatsSourceKey(item model.SiteToSiteVpnStats) string {
	if item.ID != "" {
		return prometheusLabelValuesKey([]string{
			"id",
			item.VpnID,
			item.ID,
			strconv.FormatInt(item.Spi, 10),
			item.Direction,
		})
	}

	if item.VpnID != "" || item.Spi != 0 || item.Direction != "" {
		values := []string{
			"runtime",
			item.VpnID,
			strconv.FormatInt(item.Spi, 10),
			item.Direction,
			item.LocalPeerIP,
			item.RemotePeerIP,
			item.LocalIP,
			item.RemoteIP,
			item.InterfaceName,
		}
		if item.VpnID == "" {
			values = append(values, item.Name, item.GetVpnMode(), item.GetVpnType())
		}
		return prometheusLabelValuesKey(values)
	}

	values := []string{
		"connection",
		item.Name,
		item.GetVpnMode(),
		item.GetVpnType(),
		item.LocalPeerIP,
		item.RemotePeerIP,
		item.LocalIP,
		item.RemoteIP,
		item.InterfaceName,
		strconv.Itoa(int(item.Port)),
		item.Protocol,
		item.LocalSA,
		item.RemoteSA,
	}
	return prometheusLabelValuesKey(values)
}

// shouldEmitVpnPacketSeries reports whether site-to-site tunnel stats provide enough context to expose packet metrics.
func shouldEmitVpnPacketSeries(item model.SiteToSiteVpnStats) bool {
	return item.InterfaceName != "" || item.LocalIP != "" || item.RemoteIP != "" || item.DownPkts != 0 || item.UpPkts != 0
}

// aggregateSiteToSitePeerBytes coalesces repeated snapshots of each peer by
// taking its highest counters, then sums counters across distinct peers.
func aggregateSiteToSitePeerBytes(peerStats []model.SiteToSiteVpnPeerStats, summary model.SiteToSiteVpnSummary) (int64, int64) {
	sources := make(map[string]siteToSiteTrafficValue, len(peerStats))
	for _, item := range peerStats {
		key := siteToSiteVpnPeerSourceKey(item, summary)
		source := sources[key]
		source.downBytes = max(source.downBytes, item.DownBytes)
		source.upBytes = max(source.upBytes, item.UpBytes)
		sources[key] = source
	}

	var downBytes int64
	var upBytes int64
	for _, source := range sources {
		downBytes += source.downBytes
		upBytes += source.upBytes
	}

	return downBytes, upBytes
}

// siteToSiteVpnPeerSourceKey identifies one logical peer independently from
// counters that can differ between repeated snapshots.
func siteToSiteVpnPeerSourceKey(item model.SiteToSiteVpnPeerStats, summary model.SiteToSiteVpnSummary) string {
	if peerID := siteToSitePeerID(item); peerID != "" {
		return prometheusLabelValuesKey([]string{"id", peerID})
	}

	detail := item.DetailLabels(summary)
	values := []string{
		"attributes",
		item.Name,
		item.LocalIP,
		item.RemoteIP,
		strconv.Itoa(int(item.Port)),
		detail.AllowedIPs,
	}
	return prometheusLabelValuesKey(values)
}

// siteToSiteVpnSummarySourceKey identifies the VPN that owns peer statistics.
func siteToSiteVpnSummarySourceKey(summary model.SiteToSiteVpnSummary) string {
	if summary.ID != "" {
		return prometheusLabelValuesKey([]string{"id", summary.ID})
	}
	return prometheusLabelValuesKey([]string{
		"attributes",
		summary.Name,
		summary.GetVpnType(),
		summary.GetSiteVpnType(),
	})
}

// buildSiteToSiteTunnelIDsByVpnID maps each site-to-site VPN ID to every unique
// tunnel list ID required by the peer stats endpoint.
func buildSiteToSiteTunnelIDsByVpnID(stats []model.SiteToSiteVpnStats) map[string][]string {
	tunnelIDsByVpnID := make(map[string][]string, len(stats))
	seenByVpnID := make(map[string]map[string]struct{}, len(stats))

	for _, item := range stats {
		if item.VpnID == "" || item.ID == "" {
			continue
		}
		seenTunnelIDs, exists := seenByVpnID[item.VpnID]
		if !exists {
			seenTunnelIDs = make(map[string]struct{})
			seenByVpnID[item.VpnID] = seenTunnelIDs
		}
		if _, exists := seenTunnelIDs[item.ID]; exists {
			continue
		}
		seenTunnelIDs[item.ID] = struct{}{}
		tunnelIDsByVpnID[item.VpnID] = append(tunnelIDsByVpnID[item.VpnID], item.ID)
	}

	return tunnelIDsByVpnID
}

// siteToSitePeerID returns the stable peer identifier exposed by the peer stats endpoint.
func siteToSitePeerID(item model.SiteToSiteVpnPeerStats) string {
	return firstNonEmpty(item.VpnID, item.ID)
}
