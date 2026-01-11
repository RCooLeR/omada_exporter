package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
)

func (c *DeviceCollector) collectGateway(ch chan<- prometheus.Metric, gateway *model.Gateway) error {
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
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceTxRate, prometheus.GaugeValue, gateway.TxRate, labels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceRxRate, prometheus.GaugeValue, gateway.RxRate, labels...)
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
	for _, port := range gateway.Ports {
		portLabels := append(labels,
			fmt.Sprintf("%d", port.Port),
			fmt.Sprintf("%d", port.MaxSpeed),
			port.Name,
			port.GetType(),
			port.Operation,
			port.GetLinkStatus(),
			fmt.Sprintf("%d", port.GetLinkSpeed()),
			bools.ToString(port.Poe),
			port.GetLinkSpeedLabel(),
		)
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkStatus, prometheus.GaugeValue, float64(port.LinkStatus), portLabels...)
		if port.PoePower > 0 {
			ch <- prometheus.MustNewConstMetric(c.omadaPortPowerWatts, prometheus.GaugeValue, port.PoePower, portLabels...)
		}
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkSpeedMbps, prometheus.GaugeValue, float64(port.GetLinkSpeed()), portLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkRx, prometheus.CounterValue, port.Rx, portLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkTx, prometheus.CounterValue, port.Tx, portLabels...)
	}
	return nil
}
