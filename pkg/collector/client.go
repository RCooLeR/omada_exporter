package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/pkg/api"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

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
	client                           *api.Client
}

func (c *clientCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaClientDownloadActivityBytes
	ch <- c.omadaClientUploadActivityBytes
	ch <- c.omadaClientSignalPct
	ch <- c.omadaClientSignalNoiseDbm
	ch <- c.omadaClientRssiDbm
	ch <- c.omadaClientTrafficDown
	ch <- c.omadaClientTrafficUp
	ch <- c.omadaClientTxRate
	ch <- c.omadaClientRxRate
	ch <- c.omadaClientConnectedTotal
}

func FormatWifiMode(wifiMode int) string {
	mapping := map[int]string{
		0: "802.11a",
		1: "802.11b",
		2: "802.11g",
		3: "802.11na",
		4: "802.11ng",
		5: "802.11ac",
		6: "802.11axa",
		7: "802.11axg",
		8: "802.11beg",
		9: "802.11bea",
	}
	formatted, ok := mapping[wifiMode]
	if !ok {
		return ""
	}
	return formatted
}

func (c *clientCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config

	site := config.Site
	clients, err := client.GetClients()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get clients")
		return
	}

	totals := map[string]int{}

	for _, item := range clients {
		connectionmode := "wired"
		if item.Wireless {
			connectionmode = "wireless"
		}
		vlanId := fmt.Sprintf("%.0f", item.VlanId)
		port := fmt.Sprintf("%.0f", item.Port)
		wifiMode := FormatWifiMode(int(item.WifiMode))
		labels := []string{item.Name, item.Vendor, item.Ip, item.Mac, item.HostName, site, client.SiteId, connectionmode, wifiMode, item.ApName, item.Ssid, vlanId, port, item.SwitchMac}

		if item.Wireless {
			totals[wifiMode] += 1
			ch <- prometheus.MustNewConstMetric(c.omadaClientSignalPct, prometheus.GaugeValue, item.SignalLevel, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaClientSignalNoiseDbm, prometheus.GaugeValue, item.SignalNoise, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaClientRssiDbm, prometheus.GaugeValue, item.Rssi, labels...)
		} else {
			totals["wired"] += 1
		}
		ch <- prometheus.MustNewConstMetric(c.omadaClientTrafficDown, prometheus.CounterValue, item.TrafficDown, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaClientTrafficUp, prometheus.CounterValue, item.TrafficUp, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaClientTxRate, prometheus.GaugeValue, item.TxRate, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaClientRxRate, prometheus.GaugeValue, item.RxRate, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaClientDownloadActivityBytes, prometheus.GaugeValue, item.Activity, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaClientUploadActivityBytes, prometheus.GaugeValue, item.UploadActivity, labels...)
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

func NewClientCollector(c *api.Client) *clientCollector {
	labels := []string{"client", "vendor", "ip", "mac", "host_name", "site", "site_id", "connection_mode", "wifi_mode", "ap_name", "ssid", "vlan_id", "switch_port", "switch_mac"}

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
			"TX rate of wireless client.",
			labels,
			nil,
		),

		omadaClientRxRate: prometheus.NewDesc("omada_client_rx_rate",
			"RX rate of wireless client.",
			labels,
			nil,
		),

		omadaClientConnectedTotal: prometheus.NewDesc("omada_client_connected_total",
			"Total number of connected clients.",
			[]string{"site", "site_id", "connection_mode", "wifi_mode"},
			nil,
		),

		client: c,
	}
}
