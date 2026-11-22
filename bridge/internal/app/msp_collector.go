package app

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	mspTargetLabelNames = []string{"customer", "customer_id", "site", "site_id"}

	mspTargetUpDesc = prometheus.NewDesc(
		"omada_msp_target_up",
		"Whether at least one Omada API request for this MSP customer site succeeded during its last full scrape. Unchanged when every request was served from cache.",
		mspTargetLabelNames, nil,
	)
	mspTargetDurationDesc = prometheus.NewDesc(
		"omada_msp_target_scrape_duration_seconds",
		"How long the last full scrape of this MSP customer site took.",
		mspTargetLabelNames, nil,
	)
	mspTargetAPIErrorsDesc = prometheus.NewDesc(
		"omada_msp_target_api_errors",
		"Number of failed Omada API requests (transport error, HTTP error or non-zero errorCode) during the last full scrape of this MSP customer site.",
		mspTargetLabelNames, nil,
	)
	mspTargetsDesc = prometheus.NewDesc(
		"omada_msp_targets",
		"Number of MSP customer sites currently scraped.",
		nil, nil,
	)
	mspDiscoveryLastSuccessDesc = prometheus.NewDesc(
		"omada_msp_discovery_last_success_timestamp_seconds",
		"Unix time of the last MSP discovery in which the customer list and every selected customer's site list loaded.",
		nil, nil,
	)
	mspDiscoveryFailuresDesc = prometheus.NewDesc(
		"omada_msp_discovery_failures_total",
		"Number of MSP discovery runs in which the customer list or at least one customer's site list failed to load.",
		nil, nil,
	)
)

// mspFanoutCollector scrapes every current MSP target. It is an unchecked
// collector (Describe sends nothing) because the target set, and therefore the
// constant customer labels, change at runtime. Prometheus still checks the
// gathered metrics for duplicates and label consistency.
//
// With only == "" it runs every collector of every target and adds the
// exporter's MSP self-metrics; otherwise it runs just the named collector.
type mspFanoutCollector struct {
	exporter *mspExporter
	only     string
}

func (c *mspFanoutCollector) Describe(chan<- *prometheus.Desc) {}

func (c *mspFanoutCollector) Collect(ch chan<- prometheus.Metric) {
	targets := c.exporter.snapshot()
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Go(func() {
			// One slot per target bounds the load on the controller; the
			// collectors of a target still run in parallel inside its slot.
			c.exporter.scrapeSlots <- struct{}{}
			defer func() { <-c.exporter.scrapeSlots }()
			target.collect(ch, c.only)
		})
	}
	wg.Wait()

	if c.only != "" {
		return
	}
	c.exporter.statsMu.Lock()
	lastSuccess := c.exporter.discoveryLastSuccess
	failures := c.exporter.discoveryFailures
	c.exporter.statsMu.Unlock()
	lastSuccessSeconds := 0.0
	if !lastSuccess.IsZero() {
		lastSuccessSeconds = float64(lastSuccess.UnixNano()) / 1e9
	}
	ch <- prometheus.MustNewConstMetric(mspTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
	ch <- prometheus.MustNewConstMetric(mspDiscoveryLastSuccessDesc, prometheus.GaugeValue, lastSuccessSeconds)
	ch <- prometheus.MustNewConstMetric(mspDiscoveryFailuresDesc, prometheus.CounterValue, float64(failures))
}

// collect runs the target's collectors. Collectors never return errors, so
// target health is derived from the API requests made through the target's
// own session view while they ran.
func (t *mspTarget) collect(ch chan<- prometheus.Metric, only string) {
	okBefore, failedBefore := t.session.RequestStats()
	start := time.Now()

	var wg sync.WaitGroup
	for name, collector := range t.collectors {
		if only != "" && name != only {
			continue
		}
		wg.Go(func() { collector.Collect(ch) })
	}
	wg.Wait()

	if only != "" {
		return
	}
	okAfter, failedAfter := t.session.RequestStats()
	ok, failed := okAfter-okBefore, failedAfter-failedBefore

	t.mu.Lock()
	if ok+failed > 0 {
		t.up = 0
		if ok > 0 {
			t.up = 1
		}
	}
	t.duration = time.Since(start).Seconds()
	t.lastAPIError = float64(failed)
	up, duration, apiErrors := t.up, t.duration, t.lastAPIError
	t.mu.Unlock()

	id := t.identity
	values := []string{id.CustomerName, id.CustomerID, id.SiteName, id.SiteID}
	ch <- prometheus.MustNewConstMetric(mspTargetUpDesc, prometheus.GaugeValue, up, values...)
	ch <- prometheus.MustNewConstMetric(mspTargetDurationDesc, prometheus.GaugeValue, duration, values...)
	ch <- prometheus.MustNewConstMetric(mspTargetAPIErrorsDesc, prometheus.GaugeValue, apiErrors, values...)
	t.health.Collect(ch)
}

// newMSPExporterHandler mirrors newExporterHandler for MSP mode: /metrics
// serves every target, /metrics/{collector} serves one collector of every target.
func newMSPExporterHandler(conf *config.Config, exporter *mspExporter) (http.Handler, *healthState, error) {
	mux := http.NewServeMux()
	health := &healthState{}
	registry, err := newExporterRegistry(conf.GoCollectorDisabled, conf.ProcessCollectorDisabled, conf.BridgeID)
	if err != nil {
		return nil, nil, err
	}
	registerer := bridgeRegisterer(registry, conf.BridgeID)
	if err := registerer.Register(&mspFanoutCollector{exporter: exporter}); err != nil {
		return nil, nil, fmt.Errorf("register MSP collector: %w", err)
	}
	endpoints := make(map[string]prometheus.Collector, len(exporter.collectorNames))
	for _, name := range exporter.collectorNames {
		collector := &mspFanoutCollector{exporter: exporter, only: name}
		endpoints[name] = collector
		collectorRegistry := prometheus.NewRegistry()
		collectorRegisterer := bridgeRegisterer(collectorRegistry, conf.BridgeID)
		if err := collectorRegisterer.Register(collector); err != nil {
			return nil, nil, fmt.Errorf("register %s MSP collector endpoint: %w", name, err)
		}
		mux.Handle(fmt.Sprintf("/metrics/%s", name), newMetricsHandler(collectorRegistry, collectorRegisterer))
	}
	mux.HandleFunc("/healthz", health.livez)
	mux.HandleFunc("/readyz", health.readyz)
	mux.HandleFunc("/", newExporterIndexHandler(endpoints))
	mux.Handle("/metrics", promhttp.InstrumentMetricHandler(registerer, newMetricsHandler(registry, registerer)))
	return mux, health, nil
}
