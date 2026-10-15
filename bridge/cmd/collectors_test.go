package cmd

import (
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestOpenAPICollectorsSkippedWhenOpenAPIDisabled(t *testing.T) {
	openAPIOnly := []string{"client", "vpn", "vpn-stats", "isp"}
	for _, mode := range []string{config.OpenAPIAuthDisabled, config.OpenAPIAuthClientCredentials, config.OpenAPIAuthWebSession} {
		client := &api.Client{Config: &config.Config{}, OpenAPIAuthMode: mode}
		got := initCollectors(client)
		for _, name := range openAPIOnly {
			_, exists := got[name]
			if want := mode != config.OpenAPIAuthDisabled; exists != want {
				t.Errorf("mode=%s collector %q exists=%v, want %v", mode, name, exists, want)
			}
		}
		for _, name := range []string{"controller", "alert", "device"} {
			if got[name] == nil {
				t.Errorf("mode=%s collector %q missing", mode, name)
			}
		}
	}
	for _, name := range openAPIOnly {
		if initCollectors(nil)[name] == nil {
			t.Errorf("metric documentation omits %q", name)
		}
	}
}
