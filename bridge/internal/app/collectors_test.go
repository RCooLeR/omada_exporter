package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

func TestCollectorsRespectResolvedOpenAPIAuth(t *testing.T) {
	openAPIOnly := []string{"client", "vpn", "vpn-stats", "isp", "network-quality"}
	for _, mode := range []string{config.OpenAPIAuthDisabled, config.OpenAPIAuthClientCredentials, config.OpenAPIAuthWebSession, ""} {
		t.Run("mode="+mode, func(t *testing.T) {
			client := &session.Client{Config: &config.Config{TrackInsightMetrics: true, TrackNetworkQualityMetrics: true}, OpenAPIAuthMode: mode}
			collectors := initCollectors(omada.NewClient(client))
			for _, name := range openAPIOnly {
				_, exists := collectors[name]
				if want := mode != config.OpenAPIAuthDisabled; exists != want {
					t.Errorf("collector %q exists=%v, want %v", name, exists, want)
				}
			}
			for _, name := range []string{"controller", "alert", "device", "insights"} {
				if collectors[name] == nil {
					t.Errorf("Web API collector %q is missing", name)
				}
			}
		})
	}
	for _, name := range openAPIOnly {
		if initCollectors(nil)[name] == nil {
			t.Errorf("metric documentation omits %q", name)
		}
	}
}

func TestExporterIndexOnlyAdvertisesEnabledCollectors(t *testing.T) {
	for _, mode := range []string{config.OpenAPIAuthDisabled, config.OpenAPIAuthWebSession} {
		t.Run(mode, func(t *testing.T) {
			collectors := initCollectors(omada.NewClient(&session.Client{Config: &config.Config{TrackNetworkQualityMetrics: true}, OpenAPIAuthMode: mode}))
			handler := newExporterIndexHandler(collectors)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("index status=%d", response.Code)
			}
			for name := range initCollectors(nil) {
				_, enabled := collectors[name]
				link := `href="/metrics/` + name + `"`
				if strings.Contains(response.Body.String(), link) != enabled {
					t.Errorf("index link %q does not match enabled=%v", name, enabled)
				}
			}
			for _, path := range []string{"/metrics/missing", "/unrecognized"} {
				response = httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				if response.Code != http.StatusNotFound {
					t.Errorf("unregistered path %q returned %d, want 404", path, response.Code)
				}
			}
		})
	}
}
