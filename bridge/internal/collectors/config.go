package collector

import "github.com/RCooLeR/omada_exporter/internal/api"

// includePortActivityLabel reports whether port activity label.
func includePortActivityLabel(client *api.Client) bool {
	if client == nil || client.Config == nil {
		return true
	}
	return client.Config.IncludePortActivityLabel
}

// trackPortMetrics reports whether port metrics.
func trackPortMetrics(client *api.Client) bool {
	if client == nil || client.Config == nil {
		return true
	}
	return client.Config.TrackPortMetrics
}

// trackClientMetrics reports whether client metrics.
func trackClientMetrics(client *api.Client) bool {
	if client == nil || client.Config == nil {
		return true
	}
	return client.Config.TrackClientMetrics
}

// trackInsightMetrics reports whether optional DPI insight metrics are enabled.
func trackInsightMetrics(client *api.Client) bool {
	if client == nil || client.Config == nil {
		return true
	}
	return client.Config.TrackInsightMetrics
}

// insightWindowSeconds returns the configured DPI insight query window.
func insightWindowSeconds(client *api.Client) int {
	if client == nil || client.Config == nil || client.Config.InsightWindowSeconds <= 0 {
		return 86400
	}
	return client.Config.InsightWindowSeconds
}

// insightApplicationLimit returns the configured DPI application series limit.
func insightApplicationLimit(client *api.Client) int {
	if client == nil || client.Config == nil {
		return 50
	}
	return client.Config.InsightApplicationLimit
}
