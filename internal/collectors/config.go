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
