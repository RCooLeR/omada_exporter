package collector

import (
	"github.com/charlie-haley/omada_exporter/pkg/api"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type vpnCollector struct {
	omadaVpnStatus *prometheus.Desc
	client         *api.Client
}

func (c *vpnCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaVpnStatus
}
func getPurpose(vn int8) string {
	switch vn {
	case 0:
		return "Site-to-Site"
	case 1:
		return "Client-to-Site"
	}
	return ""
}
func getVpnMode(vm int8) string {
	switch vm {
	case 0:
		return "Server"
	case 1:
		return "Client"
	}
	return ""
}
func getVpnType(vt int8) string {
	switch vt {
	case 0:
		return "L2TP"
	case 1:
		return "PPTP"
	case 2:
		return "IPSec"
	case 3:

		return "OpenVPN"
	}
	return ""
}
func (c *vpnCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config

	site := config.Site
	vpn, err := client.GetVpn()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get vpn list")
		return
	}

	for _, item := range vpn {
		purpose := getPurpose(item.Purpose)
		vpnmode := getVpnMode(item.VpnMode)
		vpntype := getVpnType(item.VpnType)
		labels := []string{item.Id, item.Name, purpose, vpnmode, vpntype, item.RemoteIp, site, client.SiteId}
		ch <- prometheus.MustNewConstMetric(c.omadaVpnStatus, prometheus.GaugeValue, bools.ToFloat64(item.Status), labels...)
	}
}

func NewVpnCollector(c *api.Client) *vpnCollector {
	labels := []string{"vpn_id", "name", "purpose", "vpn_mode", "vpn_type", "remote_ip", "site", "site_id"}

	return &vpnCollector{
		omadaVpnStatus: prometheus.NewDesc("omada_vpn_status",
			"The current status of the VPN enabled/disabled",
			labels,
			nil,
		),
		client: c,
	}
}
