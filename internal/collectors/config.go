package collector

import "github.com/RCooLeR/omada_exporter/internal/api"

func includePortActivityLabel(client *api.Client) bool {
	if client == nil || client.Config == nil {
		return true
	}
	return client.Config.IncludePortActivityLabel
}

func trackPortMetrics(client *api.Client) bool {
	if client == nil || client.Config == nil {
		return true
	}
	return client.Config.TrackPortMetrics
}

func trackClientMetrics(client *api.Client) bool {
	if client == nil || client.Config == nil {
		return true
	}
	return client.Config.TrackClientMetrics
}
