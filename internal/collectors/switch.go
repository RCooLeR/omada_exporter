package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
)

func (c *DeviceCollector) collectSwitch(ch chan<- prometheus.Metric, sw *model.Switch) error {
	labels := []string{
		sw.GetMac(),
		sw.GetType(),
		sw.GetSubtype(),
		sw.GetModel(),
		sw.GetShowModel(),
		sw.GetVersion(),
		sw.GetVersionWithUpgrade(),
		sw.GetHwVersion(),
		sw.GetFirmwareVersion(),
		sw.GetIp(),
		sw.GetName(),
		sw.GetStatus(),
		fmt.Sprintf(fmt.Sprintf("%.0f", sw.GetUptime())),
		c.webClient.Client.Config.Site,
		c.webClient.SiteId,
	}
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceTxRate, prometheus.GaugeValue, sw.TxRate, labels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceRxRate, prometheus.GaugeValue, sw.RxRate, labels...)
	if sw.PoeSupport {
		poeLabels := append(labels,
			sw.GetPoeSupport(),
			fmt.Sprintf("%d", sw.PortNumber),
			fmt.Sprintf("%d", sw.TotalPower),
		)
		ch <- prometheus.MustNewConstMetric(c.omadaDevicePoeRemainWatts, prometheus.GaugeValue, sw.PoeRemain, poeLabels...)
	}

	for _, port := range sw.Ports {
		portLabels := append(labels,
			fmt.Sprintf("%d", port.Port),
			fmt.Sprintf("%d", port.GetMaxSpeed()),
			port.Name,
			port.GetType(),
			port.Operation,
			port.PortStatus.GetLinkStatus(),
			fmt.Sprintf("%d", port.PortStatus.GetLinkSpeed()),
			bools.ToString(port.PortStatus.Poe),
			port.PortStatus.GetLinkSpeedLabel(),
		)
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkStatus, prometheus.GaugeValue, float64(port.PortStatus.LinkStatus), portLabels...)
		if sw.PoeSupport {
			ch <- prometheus.MustNewConstMetric(c.omadaPortPowerWatts, prometheus.GaugeValue, port.PortStatus.PoePower, portLabels...)
		}
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkSpeedMbps, prometheus.GaugeValue, float64(port.PortStatus.GetLinkSpeed()), portLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkRx, prometheus.CounterValue, port.PortStatus.Rx, portLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaPortLinkTx, prometheus.CounterValue, port.PortStatus.Tx, portLabels...)
	}

	for _, lag := range sw.Lags {
		lagLabels := append(labels,
			fmt.Sprintf("%d", lag.LagId),
			lag.GetLagType(),
			lag.Name,
			lag.LagStatus.GetLinkStatus(),
			fmt.Sprintf("%d", lag.LagStatus.GetLinkSpeed()),
			lag.GetPorts(),
		)
		ch <- prometheus.MustNewConstMetric(c.omadaLagLinkStatus, prometheus.GaugeValue, float64(lag.LagStatus.LinkStatus), lagLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaLagLinkSpeedMbps, prometheus.GaugeValue, float64(lag.LagStatus.GetTotalLagSpeed(sw)), lagLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaLagLinkRx, prometheus.CounterValue, lag.LagStatus.Rx, lagLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaLagLinkTx, prometheus.CounterValue, lag.LagStatus.Tx, lagLabels...)
	}
	return nil
}
