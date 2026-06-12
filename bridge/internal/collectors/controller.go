package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/webapi"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

// controllerCollector collects and exports controller metrics.
type controllerCollector struct {
	omadaControllerUptimeSeconds           *prometheus.Desc
	omadaControllerStorageUsedBytes        *prometheus.Desc
	omadaControllerStorageAvailableBytes   *prometheus.Desc
	omadaControllerStorageTotalBytes       *prometheus.Desc
	omadaControllerStorageUpgradeAvailable *prometheus.Desc
	client                                 *webapi.Client
}

const controllerStorageBytesPerGB = 1000000000

// Describe sends the collector metric descriptors to Prometheus.
func (c *controllerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaControllerUptimeSeconds
	ch <- c.omadaControllerStorageUsedBytes
	ch <- c.omadaControllerStorageAvailableBytes
	ch <- c.omadaControllerStorageTotalBytes
	ch <- c.omadaControllerStorageUpgradeAvailable
}

// Collect fetches current data and emits Prometheus metrics.
func (c *controllerCollector) Collect(ch chan<- prometheus.Metric) {
	client := c.client
	config := c.client.Config
	site := config.Site
	controller, err := client.GetController()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get controller")
		return
	}
	labels := []string{
		controller.Name,
		controller.Model,
		controller.ControllerVersion,
		controller.GetVersionWithUpgrade(),
		controller.FirmwareVersion,
		controller.MacAddress,
		controller.Ip,
		fmt.Sprintf("%d", controller.DeviceCapacity.ApCapacity),
		fmt.Sprintf("%d", controller.DeviceCapacity.AdoptedApNum),
		fmt.Sprintf("%d", controller.DeviceCapacity.OswCapacity),
		fmt.Sprintf("%d", controller.DeviceCapacity.AdoptedOswNum),
		fmt.Sprintf("%d", controller.DeviceCapacity.OsgCapacity),
		fmt.Sprintf("%d", controller.DeviceCapacity.AdoptedOsgNum),
		fmt.Sprintf("%d", controller.DeviceCapacity.OltCapacity),
		fmt.Sprintf("%d", controller.DeviceCapacity.AdoptedOltNum),
		fmt.Sprintf("%d", controller.DeviceCapacity.ApAndSwitchCapacity),
		fmt.Sprintf("%d", controller.DeviceCapacity.AdoptedApAndSwitchNum),
		bools.ToString(controller.DeviceCapacity.ShareApAndSwitchCapacity),
		site,
		client.SiteId,
	}

	ch <- prometheus.MustNewConstMetric(c.omadaControllerUptimeSeconds, prometheus.GaugeValue, controller.Uptime/1000, labels...)

	for _, s := range controller.Storage {
		storageLabels := append([]string{s.Name}, labels...)
		totalBytes := s.Total * controllerStorageBytesPerGB
		usedBytes := s.Used * controllerStorageBytesPerGB
		availableBytes := totalBytes - usedBytes
		if availableBytes < 0 {
			availableBytes = 0
		}

		ch <- prometheus.MustNewConstMetric(c.omadaControllerStorageUsedBytes, prometheus.GaugeValue, usedBytes, storageLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaControllerStorageAvailableBytes, prometheus.GaugeValue, availableBytes, storageLabels...)
		ch <- prometheus.MustNewConstMetric(c.omadaControllerStorageTotalBytes, prometheus.GaugeValue, totalBytes, storageLabels...)
	}
	for _, u := range controller.UpgradeList {
		upgradeLabels := append([]string{u.GetChannel(), u.LatestVersion}, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaControllerStorageUpgradeAvailable, prometheus.GaugeValue, bools.ToFloat64(u.UpdateAvailable), upgradeLabels...)
	}

}

// NewControllerCollector builds the Prometheus descriptors used to export controller metrics.
func NewControllerCollector(apiClient *api.Client) *controllerCollector {
	labels := []string{
		"device_name",
		"device_model",
		"device_version",
		"device_version_upgrade",
		"device_firmware_version",
		"device_mac",
		"device_ip",
		"ap_capacity",
		"adopted_ap_num",
		"osw_capacity",
		"adopted_osw_num",
		"osg_capacity",
		"adopted_osg_num",
		"olt_capacity",
		"adopted_olt_num",
		"ap_and_switch_capacity",
		"adopted_ap_and_switch_num",
		"share_ap_and_switch_capacity",
		"site",
		"site_id",
	}
	storageLabels := append([]string{"storage_name"}, labels...)
	upgradeLabels := append([]string{"upgrade_channel", "latest_version"}, labels...)
	return &controllerCollector{
		omadaControllerUptimeSeconds: prometheus.NewDesc("omada_controller_uptime_seconds",
			"Uptime of the controller.",
			labels,
			nil,
		),
		omadaControllerStorageUsedBytes: prometheus.NewDesc("omada_controller_storage_used_bytes",
			"Storage used on the controller.",
			storageLabels,
			nil,
		),
		omadaControllerStorageAvailableBytes: prometheus.NewDesc("omada_controller_storage_available_bytes",
			"Free storage available on the controller.",
			storageLabels,
			nil,
		),
		omadaControllerStorageTotalBytes: prometheus.NewDesc("omada_controller_storage_total_bytes",
			"Total storage capacity on the controller.",
			storageLabels,
			nil,
		),
		omadaControllerStorageUpgradeAvailable: prometheus.NewDesc("omada_controller_upgrade_available",
			"Firmware upgrade available for the controller per channet.",
			upgradeLabels,
			nil,
		),
		client: &webapi.Client{
			Client: apiClient,
		},
	}
}
