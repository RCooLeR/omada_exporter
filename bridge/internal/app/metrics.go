package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "github.com/prometheus/client_golang/prometheus/promhttp/zstd"
)

const (
	exporterMetricsTimeout     = 2 * time.Minute
	exporterMetricsConcurrency = 4
)

var exporterProcessStartTime = time.Now()

func newExporterRegistry(goCollectorDisabled, processCollectorDisabled bool, bridgeID string) (*prometheus.Registry, error) {
	if err := config.ValidateBridgeID(bridgeID); err != nil {
		return nil, err
	}
	registry := prometheus.NewRegistry()
	registerer := bridgeRegisterer(registry, bridgeID)
	if !goCollectorDisabled {
		if err := registerer.Register(collectors.NewGoCollector()); err != nil {
			return nil, fmt.Errorf("register Go collector: %w", err)
		}
	}
	if !processCollectorDisabled {
		if err := registerer.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
			return nil, fmt.Errorf("register process collector: %w", err)
		}
	}
	return registry, nil
}

// bridgeRegisterer isolates all exported metric families, including collector
// health and HTTP instrumentation, while leaving legacy label sets unchanged.
func bridgeRegisterer(registerer prometheus.Registerer, bridgeID string) prometheus.Registerer {
	if bridgeID == "" {
		return registerer
	}
	return prometheus.WrapRegistererWith(prometheus.Labels{"bridge_id": bridgeID}, registerer)
}

func newMetricsHandler(gatherer prometheus.Gatherer, registerer prometheus.Registerer) http.Handler {
	return promhttp.HandlerFor(gatherer, newMetricsHandlerOptions(registerer))
}

func newMetricsHandlerOptions(registerer prometheus.Registerer) promhttp.HandlerOpts {
	return promhttp.HandlerOpts{
		Registry:            registerer,
		MaxRequestsInFlight: exporterMetricsConcurrency,
		CoalesceGather:      true,
		Timeout:             exporterMetricsTimeout,
		EnableOpenMetrics:   true,
		ProcessStartTime:    exporterProcessStartTime,
	}
}
