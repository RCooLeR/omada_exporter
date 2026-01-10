package collector

import (
	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/webapi"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type alertCollector struct {
	omadaAlertNum *prometheus.Desc
	client        *webapi.Client
}

func (c *alertCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaAlertNum
}

func (c *alertCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config
	site := config.Site
	alert, err := client.GetAlert()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get controller")
		return
	}
	labels := []string{
		bools.ToString(alert.Obscured),
		site,
		client.SiteId,
	}
	ch <- prometheus.MustNewConstMetric(c.omadaAlertNum, prometheus.GaugeValue, float64(alert.AlertNum), labels...)
}

func NewAlertCollector(apiClient *api.Client) *alertCollector {
	labels := []string{
		"obscured",
		"site",
		"site_id",
	}
	return &alertCollector{
		omadaAlertNum: prometheus.NewDesc("omada_site_alert_num",
			"Number of alerts.",
			labels,
			nil,
		),
		client: &webapi.Client{
			Client: apiClient,
		},
	}
}
