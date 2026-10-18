package app

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	exporterReadHeaderTimeout   = 5 * time.Second
	exporterReadTimeout         = 10 * time.Second
	exporterIdleTimeout         = 60 * time.Second
	exporterMaxHeaderBytes      = 1 << 20
	exporterMaxHeaderValueCount = 100
)

// healthState tracks exporter readiness for the health endpoints.
type healthState struct {
	ready atomic.Bool
}

// livez reports that the process is alive.
func (h *healthState) livez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// readyz reports whether the exporter has finished startup.
func (h *healthState) readyz(w http.ResponseWriter, _ *http.Request) {
	if !h.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func newExporterHandler(conf *config.Config, collectors map[string]prometheus.Collector) (http.Handler, *healthState, error) {
	mux := http.NewServeMux()
	health := &healthState{}
	registry, err := newExporterRegistry(conf.GoCollectorDisabled, conf.ProcessCollectorDisabled, conf.BridgeID)
	if err != nil {
		return nil, nil, err
	}
	registerer := bridgeRegisterer(registry, conf.BridgeID)
	collectorHealth := newCollectorHealth()
	if err := registerer.Register(collectorHealth); err != nil {
		return nil, nil, fmt.Errorf("register collector health metrics: %w", err)
	}
	for name, collector := range collectors {
		instrumented := newInstrumentedCollector(name, collector, collectorHealth)
		collectors[name] = instrumented
		if err := registerer.Register(instrumented); err != nil {
			return nil, nil, fmt.Errorf("register %s collector: %w", name, err)
		}
		collectorRegistry := prometheus.NewRegistry()
		collectorRegisterer := bridgeRegisterer(collectorRegistry, conf.BridgeID)
		if err := collectorRegisterer.Register(instrumented); err != nil {
			return nil, nil, fmt.Errorf("register %s collector endpoint: %w", name, err)
		}
		mux.Handle(fmt.Sprintf("/metrics/%s", name), newMetricsHandler(collectorRegistry, collectorRegisterer))
	}
	mux.HandleFunc("/healthz", health.livez)
	mux.HandleFunc("/readyz", health.readyz)
	mux.HandleFunc("/", newExporterIndexHandler(collectors))
	mux.Handle("/metrics", promhttp.InstrumentMetricHandler(registerer, newMetricsHandler(registry, registerer)))
	return mux, health, nil
}

func newExporterIndexHandler(collectors map[string]prometheus.Collector) http.HandlerFunc {
	var links strings.Builder
	for _, endpoint := range []struct{ name, title string }{
		{"controller", "Controller"}, {"alert", "Alert"}, {"device", "Device"},
		{"client", "Client"}, {"vpn", "Vpn"}, {"vpn-stats", "Vpn Stats"},
		{"isp", "ISP"}, {"insights", "Insights"}, {"network-quality", "Network Quality"},
	} {
		if _, enabled := collectors[endpoint.name]; enabled {
			fmt.Fprintf(&links, `<p><a href="/metrics/%s">%s Metrics</a></p>`, endpoint.name, endpoint.title)
		}
	}
	page := fmt.Sprintf(`<html><head><title>Omada exporter</title></head><body>
<h1>Omada exporter</h1>
<p><a href="/healthz">Health</a></p>
<p><a href="/readyz">Ready</a></p>
<p><a href="/metrics">Metrics</a></p>
%s</body></html>`, links.String())
	return func(w http.ResponseWriter, r *http.Request) {
		// Missing collector routes must not masquerade as successful HTML scrapes.
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}
}

func newExporterHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:                addr,
		Handler:             handler,
		ReadHeaderTimeout:   exporterReadHeaderTimeout,
		ReadTimeout:         exporterReadTimeout,
		IdleTimeout:         exporterIdleTimeout,
		MaxHeaderBytes:      exporterMaxHeaderBytes,
		MaxHeaderValueCount: exporterMaxHeaderValueCount,
	}
}
