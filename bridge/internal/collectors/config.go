package collector

import "github.com/RCooLeR/omada_exporter/internal/omada"

// includePortActivityLabel reports whether port activity label.
func includePortActivityLabel(client *omada.Client) bool {
	if client == nil || client.Settings() == nil {
		return true
	}
	return client.Settings().IncludePortActivityLabel
}

// trackPortMetrics reports whether port metrics.
func trackPortMetrics(client *omada.Client) bool {
	if client == nil || client.Settings() == nil {
		return true
	}
	return client.Settings().TrackPortMetrics
}

// trackClientMetrics reports whether client metrics.
func trackClientMetrics(client *omada.Client) bool {
	if client == nil || client.Settings() == nil {
		return true
	}
	return client.Settings().TrackClientMetrics
}

// trackInsightMetrics reports whether optional DPI insight metrics are enabled.
func trackInsightMetrics(client *omada.Client) bool {
	if client == nil || client.Settings() == nil {
		return true
	}
	return client.Settings().TrackInsightMetrics
}

// insightWindowSeconds returns the configured DPI insight query window.
func insightWindowSeconds(client *omada.Client) int {
	if client == nil || client.Settings() == nil || client.Settings().InsightWindowSeconds <= 0 {
		return 86400
	}
	return client.Settings().InsightWindowSeconds
}

// insightApplicationLimit returns the configured DPI application series limit.
func insightApplicationLimit(client *omada.Client) int {
	if client == nil || client.Settings() == nil {
		return 50
	}
	return client.Settings().InsightApplicationLimit
}
