package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRunExporterRequiresConnectionFlags(t *testing.T) {
	conf := &config.Config{}
	err := runExporterWithConfig(context.Background(), nil, conf)
	if err == nil {
		t.Fatal("runExporter() returned nil error without required connection flags")
	}
	for _, flag := range []string{"host", "username", "password"} {
		if !strings.Contains(err.Error(), flag) {
			t.Fatalf("runExporter() error = %q, want missing %s flag", err, flag)
		}
	}
}

func TestNewExporterHTTPServerAppliesLimits(t *testing.T) {
	handler := http.NewServeMux()
	server := newExporterHTTPServer(":9202", handler)

	if server.Addr != ":9202" {
		t.Fatalf("Addr = %q, want :9202", server.Addr)
	}
	if server.Handler != handler {
		t.Fatal("Handler does not match the configured handler")
	}
	if server.ReadHeaderTimeout != exporterReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", server.ReadHeaderTimeout, exporterReadHeaderTimeout)
	}
	if server.ReadTimeout != exporterReadTimeout {
		t.Fatalf("ReadTimeout = %v, want %v", server.ReadTimeout, exporterReadTimeout)
	}
	if server.IdleTimeout != exporterIdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", server.IdleTimeout, exporterIdleTimeout)
	}
	if server.MaxHeaderBytes != exporterMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, exporterMaxHeaderBytes)
	}
	if server.MaxHeaderValueCount != exporterMaxHeaderValueCount {
		t.Fatalf("MaxHeaderValueCount = %d, want %d", server.MaxHeaderValueCount, exporterMaxHeaderValueCount)
	}
}

func TestNewExporterRegistryIsIndependentAndConfigurable(t *testing.T) {
	for range 2 {
		registry, err := newExporterRegistry(false, false)
		if err != nil {
			t.Fatalf("newExporterRegistry() error = %v", err)
		}

		families, err := registry.Gather()
		if err != nil {
			t.Fatalf("Gather() error = %v", err)
		}
		for _, name := range []string{"go_goroutines", "process_start_time_seconds"} {
			if !hasMetricFamily(families, name) {
				t.Errorf("registry is missing %q", name)
			}
		}
	}

	disabled, err := newExporterRegistry(true, true)
	if err != nil {
		t.Fatalf("newExporterRegistry(disabled) error = %v", err)
	}
	families, err := disabled.Gather()
	if err != nil {
		t.Fatalf("disabled registry Gather() error = %v", err)
	}
	if len(families) != 0 {
		t.Fatalf("disabled registry gathered %d metric families, want none", len(families))
	}
}

func TestMetricsHandlerUsesPrometheus124Features(t *testing.T) {
	options := newMetricsHandlerOptions(nil)
	if !options.CoalesceGather {
		t.Error("CoalesceGather = false, want true")
	}
	if options.MaxRequestsInFlight != exporterMetricsConcurrency {
		t.Errorf("MaxRequestsInFlight = %d, want %d", options.MaxRequestsInFlight, exporterMetricsConcurrency)
	}
	if options.Timeout != exporterMetricsTimeout {
		t.Errorf("Timeout = %v, want %v", options.Timeout, exporterMetricsTimeout)
	}
	if !options.EnableOpenMetrics {
		t.Error("EnableOpenMetrics = false, want true")
	}
	if !options.ProcessStartTime.Equal(exporterProcessStartTime) {
		t.Errorf("ProcessStartTime = %v, want %v", options.ProcessStartTime, exporterProcessStartTime)
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "omada_test_metric", Help: "Test metric."}))
	registry.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "omada_other_metric", Help: "Other metric."}))
	handler := newMetricsHandler(registry, nil)
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Accept", "application/openmetrics-text; version=1.0.0")
	request.Header.Set("Accept-Encoding", "zstd")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/openmetrics-text") {
		t.Errorf("Content-Type = %q, want OpenMetrics", contentType)
	}
	if contentEncoding := response.Header().Get("Content-Encoding"); contentEncoding != "zstd" {
		t.Errorf("Content-Encoding = %q, want zstd", contentEncoding)
	}
	wantStartTime := strconv.FormatInt(exporterProcessStartTime.Unix(), 10)
	if startTime := response.Header().Get("Process-Start-Time-Unix"); startTime != wantStartTime {
		t.Errorf("Process-Start-Time-Unix = %q, want %q", startTime, wantStartTime)
	}

	filteredRequest := httptest.NewRequest(http.MethodGet, "/metrics?name[]=omada_test_metric", nil)
	filteredResponse := httptest.NewRecorder()
	handler.ServeHTTP(filteredResponse, filteredRequest)
	if body := filteredResponse.Body.String(); !strings.Contains(body, "omada_test_metric") || strings.Contains(body, "omada_other_metric") {
		t.Errorf("filtered metrics body = %q, want only omada_test_metric", body)
	}
}

func hasMetricFamily(families []*dto.MetricFamily, name string) bool {
	for _, family := range families {
		if family.GetName() == name {
			return true
		}
	}
	return false
}
