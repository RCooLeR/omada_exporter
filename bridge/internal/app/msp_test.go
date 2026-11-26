package app

import (
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
	"github.com/RCooLeR/omada_exporter/internal/testsupport/mspfake"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func mspAppConfig(fake *mspfake.Controller) *config.Config {
	return &config.Config{
		Host:                 fake.URL(),
		Username:             fake.Username,
		Password:             fake.Password,
		ClientId:             fake.ClientID,
		SecretId:             fake.ClientSecret,
		SystemType:           config.SystemTypeAuto,
		OpenAPIAuth:          config.OpenAPIAuthAuto,
		Site:                 "Default",
		Timeout:              5,
		LogLevel:             "disabled",
		MSPDiscoveryInterval: 0,
		MSPScrapeConcurrency: 2,
		TrackClientMetrics:   true,
	}
}

func newTestMSPExporter(t *testing.T, fake *mspfake.Controller, mutate func(*config.Config)) *mspExporter {
	t.Helper()
	conf := mspAppConfig(fake)
	if mutate != nil {
		mutate(conf)
	}
	root, err := session.Configure(conf)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !root.MSPMode() {
		t.Fatal("expected MSP mode")
	}
	return newMSPExporter(conf, root)
}

func targetKeys(e *mspExporter) []string {
	var keys []string
	for _, target := range e.snapshot() {
		keys = append(keys, target.identity.key())
	}
	return keys
}

func gatherMSP(t *testing.T, e *mspExporter) map[string]*dto.MetricFamily {
	t.Helper()
	registry := prometheus.NewRegistry()
	if err := registry.Register(&mspFanoutCollector{exporter: e}); err != nil {
		t.Fatal(err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	byName := map[string]*dto.MetricFamily{}
	for _, family := range families {
		byName[family.GetName()] = family
	}
	return byName
}

func labelMap(metric *dto.Metric) map[string]string {
	labels := map[string]string{}
	for _, pair := range metric.GetLabel() {
		labels[pair.GetName()] = pair.GetValue()
	}
	return labels
}

// findMetric returns the series of family whose labels contain want.
func findMetric(family *dto.MetricFamily, want map[string]string) *dto.Metric {
	if family == nil {
		return nil
	}
	for _, metric := range family.GetMetric() {
		labels := labelMap(metric)
		matches := true
		for key, value := range want {
			if labels[key] != value {
				matches = false
				break
			}
		}
		if matches {
			return metric
		}
	}
	return nil
}

func TestMSPDiscoveryAppliesFilters(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.Config)
		want   []string
	}{
		{"all customers and sites", nil, []string{"cust-a/site-a1", "cust-a/site-a2", "cust-b/site-b1", "cust-c/site-c1"}},
		{"explicit all", func(c *config.Config) { c.MSPCustomers = "ALL"; c.Site = "*"; c.SiteSet = true }, []string{"cust-a/site-a1", "cust-a/site-a2", "cust-b/site-b1", "cust-c/site-c1"}},
		{"flag default site is not a filter", func(c *config.Config) { c.Site = "Default"; c.SiteSet = false }, []string{"cust-a/site-a1", "cust-a/site-a2", "cust-b/site-b1", "cust-c/site-c1"}},
		{"customers by name and id", func(c *config.Config) { c.MSPCustomers = " alpha corp , cust-c,unknown" }, []string{"cust-a/site-a1", "cust-a/site-a2", "cust-c/site-c1"}},
		{"sites by name", func(c *config.Config) { c.Site = "default"; c.SiteSet = true }, []string{"cust-a/site-a1", "cust-b/site-b1"}},
		{"sites by id and name", func(c *config.Config) { c.Site = "site-a2,HQ"; c.SiteSet = true }, []string{"cust-a/site-a2", "cust-c/site-c1"}},
		{"customer and site filter", func(c *config.Config) { c.MSPCustomers = "cust-a"; c.Site = "Branch"; c.SiteSet = true }, []string{"cust-a/site-a2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := mspfake.New(t)
			exporter := newTestMSPExporter(t, fake, tc.mutate)
			if err := exporter.discover(); err != nil {
				t.Fatalf("discover: %v", err)
			}
			if got := targetKeys(exporter); !slices.Equal(got, tc.want) {
				t.Fatalf("targets = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMSPMetricsCarryCustomerLabels(t *testing.T) {
	fake := mspfake.New(t)
	exporter := newTestMSPExporter(t, fake, nil)
	if err := exporter.discover(); err != nil {
		t.Fatal(err)
	}
	fake.ResetRequests()
	families := gatherMSP(t, exporter)

	for _, target := range []map[string]string{
		{"customer": "Alpha Corp", "customer_id": "cust-a", "site": "Default", "site_id": "site-a1"},
		{"customer": "Alpha Corp", "customer_id": "cust-a", "site": "Branch", "site_id": "site-a2"},
		{"customer": "Beta Ltd", "customer_id": "cust-b", "site": "Default", "site_id": "site-b1"},
		{"customer": "Gamma", "customer_id": "cust-c", "site": "HQ", "site_id": "site-c1"},
	} {
		uptime := findMetric(families["omada_controller_uptime_seconds"], target)
		if uptime == nil || uptime.GetGauge().GetValue() != 3600 {
			t.Errorf("controller uptime for %v = %v", target, uptime)
		} else if labelMap(uptime)["device_name"] != target["customer"] {
			t.Errorf("controller name = %q, want the customer name", labelMap(uptime)["device_name"])
		}
		if alert := findMetric(families["omada_site_alert_num"], target); alert == nil || alert.GetGauge().GetValue() != 2 {
			t.Errorf("alert count for %v = %v", target, alert)
		}
		if up := findMetric(families["omada_msp_target_up"], target); up == nil || up.GetGauge().GetValue() != 1 {
			t.Errorf("target up for %v = %v", target, up)
		}
		health := map[string]string{"collector": "controller"}
		for key, value := range target {
			health[key] = value
		}
		if completed := findMetric(families["omada_collector_last_scrape_completed"], health); completed == nil || completed.GetGauge().GetValue() != 1 {
			t.Errorf("collector health for %v = %v", health, completed)
		}
	}
	if targets := families["omada_msp_targets"]; targets == nil || targets.GetMetric()[0].GetGauge().GetValue() != 4 {
		t.Errorf("omada_msp_targets = %v", targets)
	}
	for _, r := range fake.Requests() {
		if strings.Contains(r.Path, "/msp-1/") && !strings.HasSuffix(r.Path, "/loginStatus") {
			t.Errorf("scrape request %s uses the MSP ID", r.Path)
		}
	}
}

func TestMSPLabelNamesAreNotUsedByCollectors(t *testing.T) {
	descs := make(chan *prometheus.Desc)
	go func() {
		for _, collector := range initCollectors(nil) {
			collector.Describe(descs)
		}
		close(descs)
	}()
	for desc := range descs {
		_, _, labels := parseDesc(desc.String())
		for label := range strings.SplitSeq(labels, ",") {
			if name := strings.TrimSpace(label); name == "customer" || name == "customer_id" {
				t.Errorf("%s already uses the %q label", desc, name)
			}
		}
		if !strings.Contains(labels, "site_id") {
			t.Errorf("%s has no site_id label; MSP targets of one customer would collide", desc)
		}
	}
}

func TestMSPFailingCustomerIsIsolated(t *testing.T) {
	fake := mspfake.New(t)
	exporter := newTestMSPExporter(t, fake, nil)
	if err := exporter.discover(); err != nil {
		t.Fatal(err)
	}
	fake.SetForbidden("cust-b", true)
	families := gatherMSP(t, exporter)

	bad := map[string]string{"customer_id": "cust-b"}
	if up := findMetric(families["omada_msp_target_up"], bad); up == nil || up.GetGauge().GetValue() != 0 {
		t.Fatalf("failing target up = %v", up)
	}
	if apiErrors := findMetric(families["omada_msp_target_api_errors"], bad); apiErrors == nil || apiErrors.GetGauge().GetValue() == 0 {
		t.Fatalf("failing target api errors = %v", apiErrors)
	}
	if uptime := findMetric(families["omada_controller_uptime_seconds"], bad); uptime != nil {
		t.Fatal("failing customer still exported controller data")
	}
	for _, healthy := range []string{"cust-a", "cust-c"} {
		want := map[string]string{"customer_id": healthy}
		if up := findMetric(families["omada_msp_target_up"], want); up == nil || up.GetGauge().GetValue() != 1 {
			t.Errorf("healthy target %s up = %v", healthy, up)
		}
		if findMetric(families["omada_controller_uptime_seconds"], want) == nil {
			t.Errorf("healthy target %s lost its metrics", healthy)
		}
	}
	if _, open := fake.Logins(); open != 1 {
		t.Errorf("OpenAPI logins = %d; the failing customer must not force re-logins", open)
	}
}

func TestMSPRediscoveryAddsAndRemovesTargets(t *testing.T) {
	fake := mspfake.New(t)
	exporter := newTestMSPExporter(t, fake, nil)
	if err := exporter.discover(); err != nil {
		t.Fatal(err)
	}
	first := exporter.snapshot()[0]

	customers := mspfake.DefaultCustomers()
	customers = append(customers[:2], mspfake.Customer{ID: "cust-d", Name: "Delta", Sites: []mspfake.Site{{ID: "site-d1", Name: "Main"}}})
	fake.SetCustomers(customers)
	if err := exporter.discover(); err != nil {
		t.Fatal(err)
	}
	want := []string{"cust-a/site-a1", "cust-a/site-a2", "cust-b/site-b1", "cust-d/site-d1"}
	if got := targetKeys(exporter); !slices.Equal(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
	if exporter.snapshot()[0] != first {
		t.Fatal("unchanged targets must be kept, not rebuilt")
	}
	families := gatherMSP(t, exporter)
	if findMetric(families["omada_controller_uptime_seconds"], map[string]string{"customer": "Delta", "site": "Main"}) == nil {
		t.Fatal("new customer is not scraped")
	}
	if findMetric(families["omada_controller_uptime_seconds"], map[string]string{"customer_id": "cust-c"}) != nil {
		t.Fatal("removed customer is still scraped")
	}

	// A failed customer listing keeps every target.
	fake.SetFailCustomerList(true)
	if err := exporter.discover(); err == nil {
		t.Fatal("expected a discovery error")
	}
	if got := targetKeys(exporter); !slices.Equal(got, want) {
		t.Fatalf("targets after failed listing = %v", got)
	}
	fake.SetFailCustomerList(false)

	// A failed site listing keeps the targets of that customer only.
	fake.SetFailSiteList("cust-a", true)
	fake.SetCustomers(append([]mspfake.Customer{{ID: "cust-a", Name: "Alpha Corp"}}, customers[2:]...))
	if err := exporter.discover(); err == nil {
		t.Fatal("expected a partial discovery error")
	}
	want = []string{"cust-a/site-a1", "cust-a/site-a2", "cust-d/site-d1"}
	if got := targetKeys(exporter); !slices.Equal(got, want) {
		t.Fatalf("targets after failed site listing = %v, want %v", got, want)
	}
	if failures := gatherMSP(t, exporter)["omada_msp_discovery_failures_total"]; failures.GetMetric()[0].GetCounter().GetValue() != 2 {
		t.Fatalf("discovery failures = %v", failures)
	}
}

func TestMSPCustomerRenameRelabelsTarget(t *testing.T) {
	fake := mspfake.New(t)
	exporter := newTestMSPExporter(t, fake, func(c *config.Config) { c.MSPCustomers = "cust-c" })
	if err := exporter.discover(); err != nil {
		t.Fatal(err)
	}
	customers := mspfake.DefaultCustomers()
	customers[2].Name = "Gamma GmbH"
	fake.SetCustomers(customers)
	if err := exporter.discover(); err != nil {
		t.Fatal(err)
	}
	if got := exporter.snapshot()[0].identity.CustomerName; got != "Gamma GmbH" {
		t.Fatalf("customer label = %q", got)
	}
}

func TestMSPHandlerServesPerCollectorEndpoints(t *testing.T) {
	fake := mspfake.New(t)
	exporter := newTestMSPExporter(t, fake, func(c *config.Config) { c.BridgeID = "msp" })
	if err := exporter.discover(); err != nil {
		t.Fatal(err)
	}
	handler, _, err := newMSPExporterHandler(exporter.conf, exporter)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) string {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		body, _ := io.ReadAll(recorder.Body)
		if recorder.Code != 200 {
			t.Fatalf("GET %s = %d: %s", path, recorder.Code, body)
		}
		return string(body)
	}
	controller := get("/metrics/controller")
	if !strings.Contains(controller, `customer="Gamma"`) || !strings.Contains(controller, `bridge_id="msp"`) {
		t.Fatalf("controller endpoint lacks labels:\n%s", controller)
	}
	if strings.Contains(controller, "omada_site_alert_num") || strings.Contains(controller, "omada_msp_target_up") {
		t.Fatal("collector endpoint must only serve its collector")
	}
	all := get("/metrics")
	for _, name := range []string{"omada_msp_target_up", "omada_site_alert_num", "omada_msp_discovery_last_success_timestamp_seconds"} {
		if !strings.Contains(all, name) {
			t.Errorf("/metrics lacks %s", name)
		}
	}
	if index := get("/"); !strings.Contains(index, "/metrics/controller") {
		t.Fatalf("index lacks collector links: %s", index)
	}
}

func TestRunMSPFailsWithoutClientCredentials(t *testing.T) {
	t.Run("auto-detected", func(t *testing.T) {
		fake := mspfake.New(t)
		conf := mspAppConfig(fake)
		conf.ClientId, conf.SecretId = "", ""
		err := Run(context.Background(), conf)
		if err == nil || !strings.Contains(err.Error(), "OMADA_CLIENT_ID") {
			t.Fatalf("Run error = %v", err)
		}
		for _, r := range fake.Requests() {
			if r.Path != "/api/info" {
				t.Fatalf("no login may be attempted without credentials, saw %s", r.Path)
			}
		}
	})
	t.Run("explicit msp fails before contacting the controller", func(t *testing.T) {
		fake := mspfake.New(t)
		conf := mspAppConfig(fake)
		conf.SystemType = config.SystemTypeMSP
		conf.SecretId = ""
		conf.MQTTEnabled = true
		err := Run(context.Background(), conf)
		if err == nil || !strings.Contains(err.Error(), "OMADA_SECRET_ID") || !strings.Contains(err.Error(), "MQTT") {
			t.Fatalf("Run error = %v", err)
		}
		if len(fake.Requests()) != 0 {
			t.Fatalf("controller was contacted: %v", fake.Requests())
		}
	})
	t.Run("auto-detected with MQTT", func(t *testing.T) {
		fake := mspfake.New(t)
		conf := mspAppConfig(fake)
		conf.MQTTEnabled = true
		err := Run(context.Background(), conf)
		if err == nil || !strings.Contains(err.Error(), "MQTT") {
			t.Fatalf("Run error = %v", err)
		}
	})
}

func TestRunMSPDumpsResponsesPerTarget(t *testing.T) {
	fake := mspfake.New(t)
	conf := mspAppConfig(fake)
	conf.MSPCustomers = "cust-a"
	conf.DumpResponsesDir = t.TempDir()
	conf.DumpResponsesOnly = true
	if err := Run(context.Background(), conf); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, err := filepath.Glob(conf.DumpResponsesDir + "/omada-*/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("manifests = %v, want one per target", entries)
	}
}
