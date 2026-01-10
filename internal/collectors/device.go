package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/webapi"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type deviceCollector struct {
	omadaDeviceUptimeSeconds *prometheus.Desc
	omadaDeviceCpuPercentage *prometheus.Desc
	omadaDeviceMemPercentage *prometheus.Desc
	omadaDeviceNeedUpgrade   *prometheus.Desc
	omadaDeviceDownload      *prometheus.Desc
	omadaDeviceUpload        *prometheus.Desc
	//gateway
	omadaWanStatus        *prometheus.Desc
	omadaWanInternetState *prometheus.Desc
	omadaWanLinkSpeedMbps *prometheus.Desc
	omadaWanRxRate        *prometheus.Desc
	omadaWanTxRate        *prometheus.Desc
	omadaWanLatency       *prometheus.Desc
	//switch
	omadaDevicePoeRemainWatts *prometheus.Desc
	omadaPortLinkStatus       *prometheus.Desc
	omadaPortPowerWatts       *prometheus.Desc
	omadaPortLinkSpeedMbps    *prometheus.Desc
	omadaPortLinkRx           *prometheus.Desc
	omadaPortLinkTx           *prometheus.Desc
	omadaLagLinkStatus        *prometheus.Desc
	omadaLagLinkSpeedMbps     *prometheus.Desc
	omadaLagLinkRx            *prometheus.Desc
	omadaLagLinkTx            *prometheus.Desc
	//ap
	omadaDeviceTxRate    *prometheus.Desc
	omadaDeviceRxRate    *prometheus.Desc
	omadaDevice2gTxUtil  *prometheus.Desc
	omadaDevice2gRxUtil  *prometheus.Desc
	omadaDevice5gTxUtil  *prometheus.Desc
	omadaDevice5gRxUtil  *prometheus.Desc
	omadaDevice5g1TxUtil *prometheus.Desc
	omadaDevice5g1RxUtil *prometheus.Desc
	omadaDevice5g2TxUtil *prometheus.Desc
	omadaDevice5g2RxUtil *prometheus.Desc
	omadaDevice6gTxUtil  *prometheus.Desc
	omadaDevice6gRxUtil  *prometheus.Desc

	webClient *webapi.Client
}

func (c *deviceCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaDeviceUptimeSeconds
	ch <- c.omadaDeviceCpuPercentage
	ch <- c.omadaDeviceMemPercentage
	ch <- c.omadaDeviceNeedUpgrade
	ch <- c.omadaDeviceDownload
	ch <- c.omadaDeviceUpload
	ch <- c.omadaWanStatus
	ch <- c.omadaWanInternetState
	ch <- c.omadaWanLinkSpeedMbps
	ch <- c.omadaWanRxRate
	ch <- c.omadaWanTxRate
	ch <- c.omadaWanLatency
	ch <- c.omadaDevicePoeRemainWatts
	ch <- c.omadaPortLinkStatus
	ch <- c.omadaPortPowerWatts
	ch <- c.omadaPortLinkSpeedMbps
	ch <- c.omadaPortLinkRx
	ch <- c.omadaPortLinkTx
	ch <- c.omadaLagLinkStatus
	ch <- c.omadaLagLinkSpeedMbps
	ch <- c.omadaLagLinkRx
	ch <- c.omadaLagLinkTx

	ch <- c.omadaDeviceTxRate
	ch <- c.omadaDeviceRxRate
	ch <- c.omadaDevice2gTxUtil
	ch <- c.omadaDevice2gRxUtil
	ch <- c.omadaDevice5gTxUtil
	ch <- c.omadaDevice5gRxUtil
	ch <- c.omadaDevice5g1TxUtil
	ch <- c.omadaDevice5g1RxUtil
	ch <- c.omadaDevice5g2TxUtil
	ch <- c.omadaDevice5g2RxUtil
	ch <- c.omadaDevice6gTxUtil
	ch <- c.omadaDevice6gRxUtil
}

func (c *deviceCollector) collectDevice(ch chan<- prometheus.Metric, device model.DeviceInterface) error {
	labels := []string{
		device.GetMac(),
		device.GetType(),
		device.GetSubtype(),
		device.GetModel(),
		device.GetShowModel(),
		device.GetVersion(),
		device.GetVersionWithUpgrade(),
		device.GetHwVersion(),
		device.GetFirmwareVersion(),
		device.GetIp(),
		device.GetName(),
		device.GetStatus(),
		fmt.Sprintf(fmt.Sprintf("%.0f", device.GetUptime())),
		c.webClient.Client.Config.Site,
		c.webClient.SiteId,
	}
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceUptimeSeconds, prometheus.GaugeValue, device.GetUptime(), labels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceCpuPercentage, prometheus.GaugeValue, device.GetCpuUtilization(), labels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceMemPercentage, prometheus.GaugeValue, device.GetMemUtilization(), labels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceNeedUpgrade, prometheus.GaugeValue, bools.ToFloat64(device.GetNeedUpgrade()), labels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceDownload, prometheus.CounterValue, device.GetDownload(), labels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDeviceUpload, prometheus.CounterValue, device.GetUpload(), labels...)
	return nil
}

func (c *deviceCollector) Collect(ch chan<- prometheus.Metric) {
	devices, err := c.webClient.GetDevices()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get devices")
		return
	}

	for _, d := range devices {
		switch item := d.(type) {
		case *model.Gateway:
			c.collectDevice(ch, item)
			c.collectGateway(ch, item)
		case *model.Switch:
			c.collectDevice(ch, item)
			c.collectSwitch(ch, item)
		case *model.AccessPoint:
			c.collectDevice(ch, item)
			c.collectAccessPoint(ch, item)
		case *model.Olt:
			c.collectDevice(ch, item)
			c.collectOlt(ch, item)
		}
	}
}

func NewDeviceCollector(apiClient *api.Client) *deviceCollector {
	deviceLabels := []string{
		"mac",
		"type",
		"subtype",
		"model",
		"show_model",
		"version",
		"version_upgrade",
		"hw_version",
		"firmware_version",
		"ip",
		"name",
		"status",
		"uptime_seconds",
		"site",
		"site_id",
	}
	gatewayWanLabels := make([]string, len(deviceLabels))
	for i, label := range deviceLabels {
		gatewayWanLabels[i] = "gateway_" + label
	}
	gatewayWanLabels = append(gatewayWanLabels,
		"port",
		"name",
		"desc",
		"type",
		"ip",
		"proto",
	)
	switchLabels := append(deviceLabels,
		"poe_support",
		"port_number",
		"total_power",
	)

	switchPortLabels := make([]string, len(switchLabels))
	for i, label := range switchLabels {
		switchPortLabels[i] = "switch_" + label
	}
	switchPortLabels = append(switchPortLabels,
		"port",
		"max_speed",
		"name",
		"type",
		"operation",
		"link_status",
		"link_speed",
		"poe",
	)

	switchLagLabels := make([]string, len(switchLabels))
	for i, label := range switchLabels {
		switchLagLabels[i] = "switch_" + label
	}
	switchLagLabels = append(switchLagLabels,
		"lag_id",
		"lag_type",
		"name",
		"link_status",
		"link_speed",
		"lag_ports",
	)

	apLabels := append(deviceLabels,
		"any_poe_enable",
		"wireless_linked",
		"wlan_group",

		"wp2g_mode",
		"wp2g_tx_max_rate",
		"wp2g_band_width",

		"wp5g_mode",
		"wp5g_tx_max_rate",
		"wp5g_band_width",

		"wp5g1_mode",
		"wp5g1_tx_max_rate",
		"wp5g1_band_width",

		"wp5g2_mode",
		"wp5g2_tx_max_rate",
		"wp5g2_band_width",

		"wp6g_mode",
		"wp6g_tx_max_rate",
		"wp6g_band_width",
	)

	return &deviceCollector{
		omadaDeviceUptimeSeconds: prometheus.NewDesc("omada_device_uptime_seconds",
			"Uptime of the device.",
			deviceLabels,
			nil,
		),
		omadaDeviceCpuPercentage: prometheus.NewDesc("omada_device_cpu_percentage",
			"Percentage of device CPU used.",
			deviceLabels,
			nil,
		),
		omadaDeviceMemPercentage: prometheus.NewDesc("omada_device_mem_percentage",
			"Percentage of device Memory used.",
			deviceLabels,
			nil,
		),
		omadaDeviceNeedUpgrade: prometheus.NewDesc("omada_device_need_upgrade",
			"A boolean on whether the device needs an upgrade.",
			deviceLabels,
			nil,
		),
		omadaDeviceDownload: prometheus.NewDesc("omada_device_download",
			"Device download traffic.",
			deviceLabels,
			nil,
		),
		omadaDeviceUpload: prometheus.NewDesc("omada_device_upload",
			"Device upload traffic.",
			deviceLabels,
			nil,
		),
		//gateway
		omadaWanStatus: prometheus.NewDesc("omada_wan_status",
			"The current status of the WAN connected/disconnected",
			gatewayWanLabels,
			nil,
		),
		omadaWanInternetState: prometheus.NewDesc("omada_wan_internet_state",
			"The current status of the WAN internet state connected/disconnected",
			gatewayWanLabels,
			nil,
		),
		omadaWanLinkSpeedMbps: prometheus.NewDesc("omada_wan_link_speed_mbps",
			"Wan link speed in mbps. This is the capability of the connection, not the active throughput.",
			gatewayWanLabels,
			nil,
		),
		omadaWanRxRate: prometheus.NewDesc("omada_wan_rx_rate",
			"Wan RX rate (KB/s)",
			gatewayWanLabels,
			nil,
		),
		omadaWanTxRate: prometheus.NewDesc("omada_wan_tx_rate",
			"Wan TX rate (KB/s)",
			gatewayWanLabels,
			nil,
		),
		omadaWanLatency: prometheus.NewDesc("omada_wan_latency",
			"Wan latency (ms)",
			gatewayWanLabels,
			nil,
		),
		//switch
		omadaDevicePoeRemainWatts: prometheus.NewDesc("omada_device_poe_remain_watts",
			"The remaining amount of PoE power for the device in watts.",
			switchLabels,
			nil,
		),
		//ports
		omadaPortLinkStatus: prometheus.NewDesc("omada_port_link_status",
			"A boolean representing the link status of the port.",
			switchPortLabels,
			nil,
		),
		omadaPortPowerWatts: prometheus.NewDesc("omada_port_power_watts",
			"The current PoE usage of the port in watts.",
			switchPortLabels,
			nil,
		),
		omadaPortLinkSpeedMbps: prometheus.NewDesc("omada_port_link_speed_mbps",
			"Port link speed in mbps. This is the capability of the connection, not the active throughput.",
			switchPortLabels,
			nil,
		),
		omadaPortLinkRx: prometheus.NewDesc("omada_port_link_rx",
			"Bytes recieved on a port.",
			switchPortLabels,
			nil,
		),
		omadaPortLinkTx: prometheus.NewDesc("omada_port_link_tx",
			"Bytes transmitted on a port.",
			switchPortLabels,
			nil,
		),
		//lag
		omadaLagLinkStatus: prometheus.NewDesc("omada_lag_link_status",
			"A boolean representing the link status of the lag.",
			switchLagLabels,
			nil,
		),
		omadaLagLinkSpeedMbps: prometheus.NewDesc("omada_lag_link_speed_mbps",
			"Lag link speed in mbps. This is the capability of the connection, not the active throughput.",
			switchLagLabels,
			nil,
		),
		omadaLagLinkRx: prometheus.NewDesc("omada_lag_link_rx",
			"Bytes recieved on a lag.",
			switchLagLabels,
			nil,
		),
		omadaLagLinkTx: prometheus.NewDesc("omada_lag_link_tx",
			"Bytes transmitted on a lag.",
			switchLagLabels,
			nil,
		),
		//ap
		omadaDeviceTxRate: prometheus.NewDesc("omada_device_tx_rate",
			"The tx rate of the device.",
			apLabels,
			nil,
		),
		omadaDeviceRxRate: prometheus.NewDesc("omada_device_rx_rate",
			"The rx rate of the device.",
			apLabels,
			nil,
		),
		omadaDevice2gTxUtil: prometheus.NewDesc("omada_device_2g_tx_util",
			"The tx rate of the device on 2.4Ghz.",
			apLabels,
			nil,
		),
		omadaDevice2gRxUtil: prometheus.NewDesc("omada_device_2g_rx_util",
			"The tx rate of the device on 2.4Ghz.",
			apLabels,
			nil,
		),
		omadaDevice5gTxUtil: prometheus.NewDesc("omada_device_5g_tx_util",
			"The tx rate of the device on 5Ghz.",
			apLabels,
			nil,
		),
		omadaDevice5gRxUtil: prometheus.NewDesc("omada_device_5g_rx_util",
			"The tx rate of the device on 5Ghz.",
			apLabels,
			nil,
		),
		omadaDevice5g1TxUtil: prometheus.NewDesc("omada_device_5g1_tx_util",
			"The tx rate of the device on 5Ghz 1.",
			apLabels,
			nil,
		),
		omadaDevice5g1RxUtil: prometheus.NewDesc("omada_device_5g1_rx_util",
			"The tx rate of the device on 5Ghz 1.",
			apLabels,
			nil,
		),
		omadaDevice5g2TxUtil: prometheus.NewDesc("omada_device_5g2_tx_util",
			"The tx rate of the device on 5Ghz 2.",
			apLabels,
			nil,
		),
		omadaDevice5g2RxUtil: prometheus.NewDesc("omada_device_5g2_rx_util",
			"The tx rate of the device on 5Ghz 2.",
			apLabels,
			nil,
		),
		omadaDevice6gTxUtil: prometheus.NewDesc("omada_device_6g_tx_util",
			"The tx rate of the device on 6Ghz..",
			apLabels,
			nil,
		),
		omadaDevice6gRxUtil: prometheus.NewDesc("omada_device_6g_rx_util",
			"The tx rate of the device on 6Ghz.",
			apLabels,
			nil,
		),
		//api-client
		webClient: &webapi.Client{
			Client: apiClient,
		},
	}
}
