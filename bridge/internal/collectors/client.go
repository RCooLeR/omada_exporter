package collector

import (
	"fmt"
	"strconv"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/openapi"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

// clientCollector collects and exports client metrics.
type clientCollector struct {
	omadaClientDownloadActivityBytes *prometheus.Desc
	omadaClientUploadActivityBytes   *prometheus.Desc
	omadaClientSignalPct             *prometheus.Desc
	omadaClientSignalNoiseDbm        *prometheus.Desc
	omadaClientRssiDbm               *prometheus.Desc
	omadaClientTrafficDown           *prometheus.Desc
	omadaClientTrafficUp             *prometheus.Desc
	omadaClientTxRate                *prometheus.Desc
	omadaClientRxRate                *prometheus.Desc
	omadaClientConnectedTotal        *prometheus.Desc
	client                           *openapi.Client
}

const clientNegotiationRateBitsPerKbit = 1000

// clientMetricLabels converts Omada's mixed client/topology response into
// unambiguous metric labels. Port and LAG values describe the client's parent
// attachment and never describe capabilities owned by the client itself.
func clientMetricLabels(item model.NetworkClient, site, siteID string) []string {
	switchMac := item.SwitchMac
	switchName := item.SwitchName
	attachmentPort := item.GetAttachmentPort()
	attachmentLagID := item.GetAttachmentLagID()
	apMac := ""
	apName := ""
	wifiMode := ""
	ssid := ""

	if item.Wireless {
		switchMac = ""
		switchName = ""
		apMac = item.ApMac
		apName = item.ApName
		wifiMode = item.GetWifiMode()
		ssid = item.Ssid
	}

	return []string{
		item.Mac,
		item.Ip,
		fmt.Sprintf("%d", item.VlanId),
		item.GetConnectType(),
		item.GetName(),
		item.SystemName,
		item.HostName,
		item.DeviceType,
		item.DeviceCategory,
		item.Vendor,
		item.ConnectDevType,
		item.GatewayMac,
		item.GatewayName,
		switchMac,
		switchName,
		attachmentPort,
		attachmentLagID,
		strconv.FormatBool(item.Wireless),
		apMac,
		apName,
		wifiMode,
		ssid,
		site,
		siteID,
	}
}

// Describe sends the collector metric descriptors to Prometheus.
func (c *clientCollector) Describe(ch chan<- *prometheus.Desc) {
	if c.trackClientMetrics() {
		ch <- c.omadaClientDownloadActivityBytes
		ch <- c.omadaClientUploadActivityBytes
		ch <- c.omadaClientSignalPct
		ch <- c.omadaClientSignalNoiseDbm
		ch <- c.omadaClientRssiDbm
		ch <- c.omadaClientTrafficDown
		ch <- c.omadaClientTrafficUp
		ch <- c.omadaClientTxRate
		ch <- c.omadaClientRxRate
	}
	ch <- c.omadaClientConnectedTotal
}

// trackClientMetrics reports whether client metrics.
func (c *clientCollector) trackClientMetrics() bool {
	return trackClientMetrics(c.client.Client)
}

// Collect fetches current data and emits Prometheus metrics.
func (c *clientCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config

	site := config.Site
	clients, err := client.GetNetworkClients()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get clients")
		return
	}

	totals := map[string]int{}

	for _, item := range clients {
		labels := clientMetricLabels(item, site, client.SiteId)

		if c.trackClientMetrics() {
			ch <- prometheus.MustNewConstMetric(c.omadaClientTrafficDown, prometheus.CounterValue, item.TrafficDown, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaClientTrafficUp, prometheus.CounterValue, item.TrafficUp, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaClientDownloadActivityBytes, prometheus.GaugeValue, item.Activity, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaClientUploadActivityBytes, prometheus.GaugeValue, item.UploadActivity, labels...)
		}

		if item.Wireless {
			totals[item.GetWifiMode()] += 1
			if c.trackClientMetrics() {
				ch <- prometheus.MustNewConstMetric(c.omadaClientRssiDbm, prometheus.GaugeValue, item.Rssi, labels...)
				ch <- prometheus.MustNewConstMetric(c.omadaClientSignalPct, prometheus.GaugeValue, item.SignalLevel, labels...)
				ch <- prometheus.MustNewConstMetric(c.omadaClientSignalNoiseDbm, prometheus.GaugeValue, item.SignalNoise, labels...)
				ch <- prometheus.MustNewConstMetric(c.omadaClientTxRate, prometheus.GaugeValue, item.TxRate*clientNegotiationRateBitsPerKbit, labels...)
				ch <- prometheus.MustNewConstMetric(c.omadaClientRxRate, prometheus.GaugeValue, item.RxRate*clientNegotiationRateBitsPerKbit, labels...)
			}
		} else {
			totals["wired"] += 1
		}
	}

	for connectionModeFmt, v := range totals {
		if connectionModeFmt == "wired" {
			ch <- prometheus.MustNewConstMetric(c.omadaClientConnectedTotal, prometheus.GaugeValue, float64(v),
				site, client.SiteId, "wired", "")
		} else {
			ch <- prometheus.MustNewConstMetric(c.omadaClientConnectedTotal, prometheus.GaugeValue, float64(v),
				site, client.SiteId, "wireless", connectionModeFmt)
		}
	}
}

// NewClientCollector builds the Prometheus descriptors used to export client metrics.
func NewClientCollector(apiClient *api.Client) *clientCollector {
	labels := []string{
		"mac",
		"ip",
		"vlan_id",
		"connect_type",
		"name",
		"system_name",
		"host_name",
		"device_type",
		"device_category",
		"vendor",

		"connect_dev_type",

		"gateway_mac",
		"gateway_name",
		"switch_mac",
		"switch_name",
		"port",
		"lag_id",

		"wireless",
		"ap_mac",
		"ap_name",
		"wifi_mode",
		"ssid",
		"site",
		"site_id",
	}

	return &clientCollector{
		omadaClientDownloadActivityBytes: prometheus.NewDesc("omada_client_download_activity_bytes",
			"The current download activity for the client in bytes.",
			labels,
			nil,
		),

		omadaClientUploadActivityBytes: prometheus.NewDesc("omada_client_upload_activity_bytes",
			"The current upload activity for the client in bytes.",
			labels,
			nil,
		),

		omadaClientSignalPct: prometheus.NewDesc("omada_client_signal_pct",
			"The signal quality for the wireless client in percent.",
			labels,
			nil,
		),

		omadaClientSignalNoiseDbm: prometheus.NewDesc("omada_client_snr_dbm",
			"The signal to noise ratio for the wireless client in dBm.",
			labels,
			nil,
		),

		omadaClientRssiDbm: prometheus.NewDesc("omada_client_rssi_dbm",
			"The RSSI for the wireless client in dBm.",
			labels,
			nil,
		),

		omadaClientTrafficDown: prometheus.NewDesc("omada_client_traffic_down_bytes",
			"Total bytes received by wireless client.",
			labels,
			nil,
		),

		omadaClientTrafficUp: prometheus.NewDesc("omada_client_traffic_up_bytes",
			"Total bytes sent by wireless client.",
			labels,
			nil,
		),

		omadaClientTxRate: prometheus.NewDesc("omada_client_tx_rate",
			"TX negotiation rate of the wireless client in bit/s.",
			labels,
			nil,
		),

		omadaClientRxRate: prometheus.NewDesc("omada_client_rx_rate",
			"RX negotiation rate of the wireless client in bit/s.",
			labels,
			nil,
		),

		omadaClientConnectedTotal: prometheus.NewDesc("omada_client_connected_total",
			"Total number of connected clients.",
			[]string{"site", "site_id", "connection_mode", "wifi_mode"},
			nil,
		),

		client: &openapi.Client{
			Client: apiClient,
		},
	}
}
