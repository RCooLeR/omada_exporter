package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/openapi"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type ispCollector struct {
	omadaIspStatus        *prometheus.Desc
	omadaIspDownloadSpeed *prometheus.Desc
	omadaIspUploadSpeed   *prometheus.Desc
	client                *openapi.Client
}

func (c *ispCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaIspStatus
	ch <- c.omadaIspDownloadSpeed
	ch <- c.omadaIspUploadSpeed
}
func (c *ispCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config

	site := config.Site
	isp, err := client.GetIsp()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get ISP list")
		return
	}

	for _, item := range isp {
		labels := []string{
			item.GatewayName,
			item.GatewayMac,
			item.GetGatewayStatus(),
			item.Name,
			fmt.Sprintf("%d", item.Port),
			item.GetStatus(),
			item.IP,
			item.LoadBalance,
			fmt.Sprintf("%d", item.MaxBandwidth),
			fmt.Sprintf("%d", item.DownloadSpeedSet),
			site,
			client.SiteId,
		}
		ch <- prometheus.MustNewConstMetric(c.omadaIspStatus, prometheus.GaugeValue, float64(item.Status), labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaIspDownloadSpeed, prometheus.GaugeValue, item.DownloadSpeed, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaIspUploadSpeed, prometheus.GaugeValue, item.UploadSpeed, labels...)
	}
}

func NewISPCollector(apiClient *api.Client) *ispCollector {
	labels := []string{
		"gateway_name",
		"gateway_mac",
		"gateway_status",
		//Labels
		"name",
		"port",
		"status",
		"ip",
		"load_balance",
		"max_bandwidth",
		"download_speed_set",
		"site",
		"site_id",
	}

	return &ispCollector{
		omadaIspStatus: prometheus.NewDesc("omada_isp_status",
			"The current status of the ISP enabled/disabled",
			labels,
			nil,
		),
		omadaIspDownloadSpeed: prometheus.NewDesc("omada_isp_download_speed",
			"The download speed of the ISP",
			labels,
			nil,
		),
		omadaIspUploadSpeed: prometheus.NewDesc("omada_isp_upload_speed",
			"The upload speed of the ISP",
			labels,
			nil,
		),
		client: &openapi.Client{
			Client: apiClient,
		},
	}
}
