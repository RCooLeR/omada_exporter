package collector

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/webapi"
	"github.com/goki/ki/bools"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

type controllerCollector struct {
	omadaControllerUptimeSeconds           *prometheus.Desc
	omadaControllerStorageUsedBytes        *prometheus.Desc
	omadaControllerStorageAvailableBytes   *prometheus.Desc
	omadaControllerStorageUpgradeAvailable *prometheus.Desc
	client                                 *webapi.Client
}

func (c *controllerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaControllerUptimeSeconds
	ch <- c.omadaControllerStorageUsedBytes
	ch <- c.omadaControllerStorageAvailableBytes
	ch <- c.omadaControllerStorageUpgradeAvailable
}

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
		fmt.Sprintf(fmt.Sprintf("%.0f", controller.Uptime/1000)),
		site,
		client.SiteId,
	}

	ch <- prometheus.MustNewConstMetric(c.omadaControllerUptimeSeconds, prometheus.GaugeValue, controller.Uptime/1000, labels...)

	for _, s := range controller.Storage {
		storageLabels := append([]string{s.Name}, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaControllerStorageUsedBytes, prometheus.GaugeValue, s.Used*1000000000, storageLabels...)

		ch <- prometheus.MustNewConstMetric(c.omadaControllerStorageAvailableBytes, prometheus.GaugeValue, s.Total*100000000, storageLabels...)
	}
	for _, u := range controller.UpgradeList {
		upgradeLabels := append([]string{u.GetChannel(), u.LatestVersion}, labels...)
		ch <- prometheus.MustNewConstMetric(c.omadaControllerStorageUpgradeAvailable, prometheus.GaugeValue, bools.ToFloat64(u.UpdateAvailable), upgradeLabels...)
	}

}

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
		"device_uptime_seconds",
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
			"Total storage available for the controller.",
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
