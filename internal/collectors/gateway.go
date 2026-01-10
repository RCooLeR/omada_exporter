package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/prometheus/client_golang/prometheus"
)

func (c *deviceCollector) collectGateway(ch chan<- prometheus.Metric, gateway *model.Gateway) error {
	labels := []string{
		gateway.GetMac(),
		gateway.GetType(),
		gateway.GetSubtype(),
		gateway.GetModel(),
		gateway.GetShowModel(),
		gateway.GetVersion(),
		gateway.GetVersionWithUpgrade(),
		gateway.GetHwVersion(),
		gateway.GetFirmwareVersion(),
		gateway.GetIp(),
		gateway.GetName(),
		gateway.GetStatus(),
		fmt.Sprintf(fmt.Sprintf("%.0f", gateway.GetUptime())),
		c.webClient.Client.Config.Site,
		c.webClient.SiteId,
	}
	for _, wan := range gateway.Wans {
		wanLabels := append(labels,
			fmt.Sprintf("%d", wan.Port),
			wan.Name,
			wan.Desc,
			wan.GetType(),
			wan.Ip,
			wan.Proto,
		)
		ch <- prometheus.MustNewConstMetric(c.omadaWanStatus, prometheus.GaugeValue, float64(wan.Status), wanLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaWanInternetState, prometheus.GaugeValue, float64(wan.InternetState), wanLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaWanLinkSpeedMbps, prometheus.GaugeValue, float64(wan.GetLinkSpeed()), wanLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaWanRxRate, prometheus.GaugeValue, wan.RxRate, wanLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaWanTxRate, prometheus.GaugeValue, wan.TxRate, wanLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaWanLatency, prometheus.GaugeValue, float64(wan.Latency), wanLabels...)
	}
	return nil
}
