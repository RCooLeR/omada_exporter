package collector

import (
	"fmt"
	"sort"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/webapi"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/rs/zerolog/log"
)

// insightsCollector collects optional DPI insight metrics.
type insightsCollector struct {
	omadaDPITotalTrafficBytes       *prometheus.Desc
	omadaDPICategoryTrafficBytes    *prometheus.Desc
	omadaDPIApplicationTrafficBytes *prometheus.Desc
	omadaDPIScrapeWindowSeconds     *prometheus.Desc
	client                          *webapi.Client
}

// Describe sends the collector metric descriptors to Prometheus.
func (c *insightsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.omadaDPITotalTrafficBytes
	ch <- c.omadaDPICategoryTrafficBytes
	ch <- c.omadaDPIApplicationTrafficBytes
	ch <- c.omadaDPIScrapeWindowSeconds
}

// Collect fetches current DPI insight data and emits Prometheus metrics.
func (c *insightsCollector) Collect(ch chan<- prometheus.Metric) {
	if !trackInsightMetrics(c.client.Client) {
		return
	}

	client := c.client
	site := client.Config.Site
	siteLabels := []string{site, client.SiteId}

	insights, err := client.GetDPIInsights(insightWindowSeconds(client.Client))
	if err != nil {
		log.Error().Err(err).Msg("Failed to get DPI insights")
		return
	}

	ch <- prometheus.MustNewConstMetric(c.omadaDPIScrapeWindowSeconds, prometheus.GaugeValue, float64(insights.WindowSeconds), siteLabels...)
	ch <- prometheus.MustNewConstMetric(c.omadaDPITotalTrafficBytes, prometheus.GaugeValue, insights.TotalTraffic, siteLabels...)

	for _, category := range insights.Categories {
		labels := []string{
			fmt.Sprintf("%d", category.FamilyID),
			category.FamilyName,
			site,
			client.SiteId,
		}
		ch <- prometheus.MustNewConstMetric(c.omadaDPICategoryTrafficBytes, prometheus.GaugeValue, category.Traffic, labels...)
	}

	for _, app := range limitedDPIApplications(insights.Applications, insightApplicationLimit(client.Client)) {
		labels := []string{
			fmt.Sprintf("%d", app.FamilyID),
			app.FamilyName,
			fmt.Sprintf("%d", app.ApplicationID),
			app.ApplicationName,
			site,
			client.SiteId,
		}
		ch <- prometheus.MustNewConstMetric(c.omadaDPIApplicationTrafficBytes, prometheus.GaugeValue, app.Traffic, labels...)
	}
}

// NewInsightsCollector builds the Prometheus descriptors used to export DPI insights.
func NewInsightsCollector(apiClient *api.Client) *insightsCollector {
	siteLabels := []string{"site", "site_id"}
	categoryLabels := []string{"family_id", "family_name", "site", "site_id"}
	applicationLabels := []string{"family_id", "family_name", "application_id", "application_name", "site", "site_id"}

	return &insightsCollector{
		omadaDPITotalTrafficBytes: prometheus.NewDesc("omada_dpi_total_traffic_bytes",
			"Total DPI-classified traffic in bytes for the configured insight window.",
			siteLabels,
			nil,
		),
		omadaDPICategoryTrafficBytes: prometheus.NewDesc("omada_dpi_category_traffic_bytes",
			"DPI-classified traffic in bytes by category for the configured insight window.",
			categoryLabels,
			nil,
		),
		omadaDPIApplicationTrafficBytes: prometheus.NewDesc("omada_dpi_application_traffic_bytes",
			"DPI-classified traffic in bytes by application for the configured insight window. Omada may not attribute every byte to an application.",
			applicationLabels,
			nil,
		),
		omadaDPIScrapeWindowSeconds: prometheus.NewDesc("omada_dpi_scrape_window_seconds",
			"DPI insight query window in seconds.",
			siteLabels,
			nil,
		),
		client: &webapi.Client{
			Client: apiClient,
		},
	}
}

func limitedDPIApplications(apps []model.DPIApplicationTraffic, limit int) []model.DPIApplicationTraffic {
	if limit == 0 || len(apps) == 0 {
		return nil
	}

	sortedApps := append([]model.DPIApplicationTraffic(nil), apps...)
	sort.SliceStable(sortedApps, func(i, j int) bool {
		return sortedApps[i].Traffic > sortedApps[j].Traffic
	})

	if limit > 0 && len(sortedApps) > limit {
		return sortedApps[:limit]
	}
	return sortedApps
}
