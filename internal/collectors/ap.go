package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
)

func (c *DeviceCollector) collectAccessPoint(ch chan<- prometheus.Metric, ap *model.AccessPoint) error {
	deviceLabels := []string{
		ap.GetMac(),
		ap.GetType(),
		ap.GetSubtype(),
		ap.GetModel(),
		ap.GetShowModel(),
		ap.GetVersion(),
		ap.GetVersionWithUpgrade(),
		ap.GetHwVersion(),
		ap.GetFirmwareVersion(),
		ap.GetIp(),
		ap.GetName(),
		ap.GetStatus(),
		fmt.Sprintf(fmt.Sprintf("%.0f", ap.GetUptime())),
		c.webClient.Client.Config.Site,
		c.webClient.SiteId,
	}
	labels := append(deviceLabels,
		bools.ToString(ap.AnyPoeEnable),
		bools.ToString(ap.WirelessLinked),
		ap.WlanGroup,
	)
	if ap.Wp2GHz != nil {
		labels = append(labels,
			ap.Wp2GHz.RdMode,
			fmt.Sprintf("%d", ap.Wp2GHz.MaxTxRate),
			ap.Wp2GHz.BandWidth,
		)
	} else {
		labels = append(labels,
			"",
			"",
			"",
		)
	}
	if ap.Wp5GHz != nil {
		labels = append(labels,
			ap.Wp5GHz.RdMode,
			fmt.Sprintf("%d", ap.Wp5GHz.MaxTxRate),
			ap.Wp5GHz.BandWidth,
		)
	} else {
		labels = append(labels,
			"",
			"",
			"",
		)
	}
	if ap.Wp5GHz_1 != nil {
		labels = append(labels,
			ap.Wp5GHz.RdMode,
			fmt.Sprintf("%d", ap.Wp5GHz_1.MaxTxRate),
			ap.Wp5GHz_1.BandWidth,
		)
	} else {
		labels = append(labels,
			"",
			"",
			"")
	}
	if ap.Wp5GHz_2 != nil {
		labels = append(labels,
			ap.Wp5GHz.RdMode,
			fmt.Sprintf("%d", ap.Wp5GHz_2.MaxTxRate),
			ap.Wp5GHz_2.BandWidth,
		)
	} else {
		labels = append(labels,
			"",
			"",
			"",
		)
	}
	if ap.Wp6GHz != nil {
		labels = append(labels,
			ap.Wp6GHz.RdMode,
			fmt.Sprintf("%d", ap.Wp6GHz.MaxTxRate),
			ap.Wp6GHz.BandWidth,
		)
	} else {
		labels = append(labels,
			"",
			"",
			"",
		)
	}

	ch <- prometheus.MustNewConstMetric(c.omadaDeviceTxRate, prometheus.GaugeValue, ap.TxRate, deviceLabels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceRxRate, prometheus.GaugeValue, ap.RxRate, deviceLabels...)
	if ap.Wp2GHz != nil {
		ch <- prometheus.MustNewConstMetric(c.omadaDevice2gTxUtil, prometheus.GaugeValue, ap.Wp2GHz.TxUtilization, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaDevice2gRxUtil, prometheus.GaugeValue, ap.Wp2GHz.RxUtilization, labels...)
	}
	if ap.Wp5GHz != nil {
		ch <- prometheus.MustNewConstMetric(c.omadaDevice5gTxUtil, prometheus.GaugeValue, ap.Wp5GHz.TxUtilization, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaDevice5gRxUtil, prometheus.GaugeValue, ap.Wp5GHz.RxUtilization, labels...)
	}
	if ap.Wp5GHz_1 != nil {
		ch <- prometheus.MustNewConstMetric(c.omadaDevice5g1TxUtil, prometheus.GaugeValue, ap.Wp5GHz_1.TxUtilization, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaDevice5g1RxUtil, prometheus.GaugeValue, ap.Wp5GHz_1.RxUtilization, labels...)
	}
	if ap.Wp5GHz_2 != nil {
		ch <- prometheus.MustNewConstMetric(c.omadaDevice5g2TxUtil, prometheus.GaugeValue, ap.Wp5GHz_2.TxUtilization, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaDevice5g2RxUtil, prometheus.GaugeValue, ap.Wp5GHz_2.RxUtilization, labels...)
	}
	if ap.Wp6GHz != nil {
		ch <- prometheus.MustNewConstMetric(c.omadaDevice6gTxUtil, prometheus.GaugeValue, ap.Wp6GHz.TxUtilization, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaDevice6gRxUtil, prometheus.GaugeValue, ap.Wp6GHz.RxUtilization, labels...)
	}
	return nil
}
