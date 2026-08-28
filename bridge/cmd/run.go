package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/hamqtt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "github.com/prometheus/client_golang/prometheus/promhttp/zstd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

const (
	exporterReadHeaderTimeout   = 5 * time.Second
	exporterReadTimeout         = 10 * time.Second
	exporterIdleTimeout         = 60 * time.Second
	exporterShutdownTimeout     = 10 * time.Second
	exporterMQTTShutdownTimeout = 15 * time.Second
	exporterMetricsTimeout      = 2 * time.Minute
	exporterMetricsConcurrency  = 4
	exporterMaxHeaderBytes      = 1 << 20
	exporterMaxHeaderValueCount = 100
)

var exporterProcessStartTime = time.Now()

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

// runExporterWithConfig configures the exporter and starts the HTTP server.
func runExporterWithConfig(ctx context.Context, _ *cli.Command, conf *config.Config) error {
	if conf == nil {
		return fmt.Errorf("exporter configuration is nil")
	}
	missing := make([]string, 0, 3)
	if conf.Host == "" {
		missing = append(missing, "host")
	}
	if conf.Username == "" {
		missing = append(missing, "username")
	}
	if conf.Password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required flags %q not set", strings.Join(missing, ", "))
	}

	// set log level
	level, err := zerolog.ParseLevel(conf.LogLevel)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(level)

	// check if host is properly formatted
	if strings.HasSuffix(conf.Host, "/") {
		// remove trailing slash if it exists
		conf.Host = strings.TrimRight(conf.Host, "/")
	}

	client, err := api.Configure(conf)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	health := &healthState{}
	registry, err := newExporterRegistry(conf.GoCollectorDisabled, conf.ProcessCollectorDisabled)
	if err != nil {
		return err
	}

	collectors := initCollectors(client)
	collectorHealth := newCollectorHealth()
	if err := registry.Register(collectorHealth); err != nil {
		return fmt.Errorf("register collector health metrics: %w", err)
	}

	// register omada collectors
	for name, c := range collectors {
		instrumented := newInstrumentedCollector(name, c, collectorHealth)
		collectors[name] = instrumented
		if err := registry.Register(instrumented); err != nil {
			return fmt.Errorf("register %s collector: %w", name, err)
		}

		collectorRegistry := prometheus.NewRegistry()
		if err := collectorRegistry.Register(instrumented); err != nil {
			return fmt.Errorf("register %s collector endpoint: %w", name, err)
		}
		mux.Handle(fmt.Sprintf("/metrics/%s", name), newMetricsHandler(collectorRegistry, collectorRegistry))
	}

	var publisherDone <-chan struct{}
	if conf.MQTTEnabled {
		publisher, err := hamqtt.NewPublisher(client, collectors)
		if err != nil {
			log.Error().Err(err).Msg("home assistant mqtt publisher disabled")
		} else {
			done := make(chan struct{})
			publisherDone = done
			go func() {
				defer close(done)
				if err := publisher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
					log.Error().Err(err).Msg("home assistant mqtt publisher stopped")
				}
			}()
		}
	}

	insightsLink := ""
	if conf.TrackInsightMetrics {
		insightsLink = `<p>
				<a href="/metrics/insights">Insights Metrics</a>
			</p>`
	}

	mux.HandleFunc("/healthz", health.livez)
	mux.HandleFunc("/readyz", health.readyz)
	log.Info().Msg(fmt.Sprintf("listening on :%s", conf.Port))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`<html>
    <head>
	<title>Omada exporter</title>
	</head>
    	<body>
			<h1>Omada exporter</h1>
			<p>
				<a href="/healthz">Health</a>
			</p>
			<p>
				<a href="/readyz">Ready</a>
			</p>
			<p>
				<a href="/metrics">Metrics</a>
			</p>
			<p>
				<a href="/metrics/controller">Controller Metrics</a>
			</p>
			<p>
				<a href="/metrics/alert">Alert Metrics</a>
			</p>
			<p>
				<a href="/metrics/device">Device Metrics</a>
			</p>
			<p>
				<a href="/metrics/client">Client Metrics</a>
			</p>
			<p>
				<a href="/metrics/vpn">Vpn Metrics</a>
			</p>
			<p>
				<a href="/metrics/vpn-stats">Vpn Stats Metrics</a>
			</p>
			<p>
				<a href="/metrics/isp">ISP Metrics</a>
			</p>
			%s
    	</body>
    </html>`, insightsLink)))
	})

	metricsHandler := newMetricsHandler(registry, registry)
	mux.Handle("/metrics", promhttp.InstrumentMetricHandler(registry, metricsHandler))
	health.ready.Store(true)

	server := newExporterHTTPServer(fmt.Sprintf(":%s", conf.Port), mux)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), exporterShutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		waitForPublisher(publisherDone, exporterMQTTShutdownTimeout)
		if shutdownErr != nil {
			return fmt.Errorf("shut down exporter HTTP server: %w", shutdownErr)
		}

		err := <-serveErr
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func newExporterRegistry(goCollectorDisabled, processCollectorDisabled bool) (*prometheus.Registry, error) {
	registry := prometheus.NewRegistry()
	if !goCollectorDisabled {
		if err := registry.Register(collectors.NewGoCollector()); err != nil {
			return nil, fmt.Errorf("register Go collector: %w", err)
		}
	}
	if !processCollectorDisabled {
		if err := registry.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
			return nil, fmt.Errorf("register process collector: %w", err)
		}
	}
	return registry, nil
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

func waitForPublisher(done <-chan struct{}, timeout time.Duration) {
	if done == nil {
		return
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		log.Warn().Dur("timeout", timeout).Msg("timed out waiting for MQTT publisher shutdown")
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
