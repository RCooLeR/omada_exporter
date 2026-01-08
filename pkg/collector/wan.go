package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/pkg/api"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type wanCollector struct {
	omadaWanStatus        *prometheus.Desc
	omadaWanInternetState *prometheus.Desc
	omadaWanLinkSpeedMbps *prometheus.Desc
	omadaWanRxRate        *prometheus.Desc
	omadaWanTxRate        *prometheus.Desc
	omadaWanLatency       *prometheus.Desc
	client                *api.Client
}

func removeWanDuplicates(s []api.Wan) []api.Wan {
	// create map to track found items
	found := map[api.Wan]bool{}
	res := []api.Wan{}

	for v := range s {
		if found[s[v]] {
			// skip adding to new array if it exists
			continue
		}
		// add to new array, mark as found
		found[s[v]] = true
		res = append(res, s[v])
	}
	return res
}
func (c *wanCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaWanStatus
	ch <- c.omadaWanInternetState
	ch <- c.omadaWanLinkSpeedMbps
	ch <- c.omadaWanRxRate
	ch <- c.omadaWanTxRate
	ch <- c.omadaWanLatency
}
func getWanType(wt int8) string {
	switch wt {
	case 0:
		return "WAN"
	case 1:
		return "WAN/LAN"
	case 2:
		return "LAN"
	}
	return "WAN"
}
func (c *wanCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config

	site := config.Site
	devices, err := client.GetDevices()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get wan stats")
		return
	}

	for _, device := range devices {
		// The Omada exporter sometimes returns duplicate ports. e.g an 8 port switch will return 16 ports with identical ports
		// this causes issues with Prometheus as it tries to register duplicate metrics. A bit of hacky fix, but here we remove
		// duplicate ports to prevent this error.
		wans := removeWanDuplicates(device.Wans)
		for _, w := range wans {
			linkSpeed := getPortByLinkSpeed(w.LinkSpeed)
			port := fmt.Sprintf("%.0f", w.Port)
			wtype := getWanType(w.Type)
			labels := []string{device.Name, device.Mac, port, w.Name, w.Desc, wtype, w.Ip, w.Proto, site, client.SiteId}

			ch <- prometheus.MustNewConstMetric(c.omadaWanStatus, prometheus.GaugeValue, float64(w.Status), labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaWanInternetState, prometheus.GaugeValue, float64(w.InternetState), labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaWanLinkSpeedMbps, prometheus.GaugeValue, linkSpeed, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaWanRxRate, prometheus.GaugeValue, w.RxRate, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaWanTxRate, prometheus.GaugeValue, w.TxRate, labels...)
			ch <- prometheus.MustNewConstMetric(c.omadaWanLatency, prometheus.GaugeValue, float64(w.Latency), labels...)

		}
	}
}
func NewWanCollector(c *api.Client) *wanCollector {
	labels := []string{"device", "device_mac", "port", "name", "desc", "type", "ip", "proto", "site", "site_id"}

	return &wanCollector{
		omadaWanStatus: prometheus.NewDesc("omada_wan_status",
			"The current status of the WAN connected/disconnected",
			labels,
			nil,
		),
		omadaWanInternetState: prometheus.NewDesc("omada_wan_internet_state",
			"The current status of the WAN internet state connected/disconnected",
			labels,
			nil,
		),
		omadaWanLinkSpeedMbps: prometheus.NewDesc("omada_wan_link_speed_mbps",
			"Wan link speed in mbps. This is the capability of the connection, not the active throughput.",
			labels,
			nil,
		),
		omadaWanRxRate: prometheus.NewDesc("omada_wan_rx_rate",
			"Wan RX rate (KB/s)",
			labels,
			nil,
		),
		omadaWanTxRate: prometheus.NewDesc("omada_wan_tx_rate",
			"Wan TX rate (KB/s)",
			labels,
			nil,
		),
		omadaWanLatency: prometheus.NewDesc("omada_wan_latency",
			"Wan latency (ms)",
			labels,
			nil,
		),
		client: c,
	}
}
