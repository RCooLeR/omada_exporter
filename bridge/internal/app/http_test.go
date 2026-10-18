package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

func TestExporterHandlerPreservesHealthAndScopedMetrics(t *testing.T) {
	collectors := map[string]prometheus.Collector{
		"device": newConstCollector("omada_test_device", 1),
		"alert":  newConstCollector("omada_test_alert", 2),
	}
	handler, health, err := newExporterHandler(&config.Config{
		BridgeID: "test-bridge", GoCollectorDisabled: true, ProcessCollectorDisabled: true,
	}, collectors)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		code int
	}{
		{"/healthz", http.StatusOK},
		{"/readyz", http.StatusServiceUnavailable},
		{"/metrics/missing", http.StatusNotFound},
		{"/unknown", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.code {
			t.Errorf("%s status = %d, want %d", test.path, response.Code, test.code)
		}
	}
	health.ready.Store(true)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Errorf("ready status = %d", response.Code)
	}
	for _, path := range []string{"/metrics/device", "/metrics"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, response.Code)
		}
		body := response.Body.String()
		if !strings.Contains(body, `omada_test_device{bridge_id="test-bridge"} 1`) {
			t.Errorf("%s is missing the scoped device metric", path)
		}
		if all := path == "/metrics"; strings.Contains(body, "omada_test_alert") != all {
			t.Errorf("%s did not preserve per-collector metric isolation", path)
		}
	}
}
