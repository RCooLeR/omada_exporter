package collector

import (
	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/openapi"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type vpnCollector struct {
	omadaVpnStatus *prometheus.Desc
	client         *openapi.Client
}

func (c *vpnCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaVpnStatus
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
		labels := []string{item.Id, item.Name, item.GetPurpose(), item.GetVpnMode(), item.GetVpnType(), item.RemoteIp, site, client.SiteId}
		ch <- prometheus.MustNewConstMetric(c.omadaVpnStatus, prometheus.GaugeValue, bools.ToFloat64(item.Status), labels...)
	}
}

func NewVpnCollector(apiClient *api.Client) *vpnCollector {
	labels := []string{"vpn_id", "name", "purpose", "vpn_mode", "vpn_type", "remote_ip", "site", "site_id"}

	return &vpnCollector{
		omadaVpnStatus: prometheus.NewDesc("omada_vpn_status",
			"The current status of the VPN enabled/disabled",
			labels,
			nil,
		),
		client: &openapi.Client{
			Client: apiClient,
		},
	}
}
