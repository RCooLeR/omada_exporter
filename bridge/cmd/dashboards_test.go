package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	dashboardDatasourceUID = "$datasource"
	dashboardRateMinStep   = "1m"
)

var scrapeLabels = map[string]struct{}{"instance": {}, "job": {}}

var (
	metricNamePattern           = regexp.MustCompile("\\bomada_[a-zA-Z0-9_]+\\b")
	matcherPattern              = regexp.MustCompile("(^|,)\\s*([a-zA-Z_][a-zA-Z0-9_]*)\\s*(=~|!~|!=|=)")
	groupByPattern              = regexp.MustCompile("\\b(sum|avg|max|min|count)\\s+by\\s*\\(([^)]*)\\)")
	legendPattern               = regexp.MustCompile("\\{\\{\\s*([a-zA-Z_][a-zA-Z0-9_]*)\\s*\\}\\}")
	fixedRangePattern           = regexp.MustCompile("\\[[0-9]+[smhdwy]\\]")
	exactVariableMatcherPattern = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*\s*=\s*"\$(?:\{[^}]+\}|[a-zA-Z_][a-zA-Z0-9_]*)"`)
	literalDeviceMACPattern     = regexp.MustCompile(`device_mac\s*(?:=|=~)\s*"[0-9A-Fa-f]{2}(?:[:-][0-9A-Fa-f]{2}){5}"`)
)

type grafanaDashboard struct {
	Description   string
	ID            *int
	Panels        []grafanaPanel
	Refresh       string
	SchemaVersion int
	Templating    struct {
		List []grafanaVariable
	}
	Title string
	UID   string
}

type grafanaDatasource struct {
	Type string
	UID  string
}

type grafanaVariable struct {
	AllValue   string
	Datasource *grafanaDatasource
	IncludeAll bool
	Multi      bool
	Name       string
	Query      json.RawMessage
	Refresh    int
	Regex      string
	Type       string
}

type grafanaPanel struct {
	Datasource  *grafanaDatasource
	Description string
	FieldConfig json.RawMessage
	GridPos     struct {
		H int
		W int
		X int
		Y int
	}
	ID      int
	Panels  []grafanaPanel
	Targets []grafanaTarget
	Title   string
	Type    string
}

type grafanaTarget struct {
	Datasource   *grafanaDatasource
	Expr         string
	Instant      bool
	Interval     string
	LegendFormat string
	Range        bool
	RefID        string
}

type prometheusSelector struct {
	metric   string
	matchers string
}

func TestGrafanaDashboardsMatchExporterContract(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "docs", "dashboards", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 3 {
		t.Fatalf("dashboard count = %d, want at least 3", len(paths))
	}
	slices.Sort(paths)

	contract := dashboardMetricContract(t)
	uids := make(map[string]string, len(paths))

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			var dashboard grafanaDashboard
			if err := json.Unmarshal(data, &dashboard); err != nil {
				t.Fatalf("parse dashboard: %v", err)
			}
			if dashboard.ID != nil {
				t.Errorf("id = %d; distributable dashboards must use null", *dashboard.ID)
			}
			if strings.TrimSpace(dashboard.Title) == "" {
				t.Error("dashboard title is empty")
			}
			if strings.TrimSpace(dashboard.Description) == "" {
				t.Error("dashboard description is empty")
			}
			if dashboard.UID == "" {
				t.Error("dashboard UID is empty")
			} else if previous, exists := uids[dashboard.UID]; exists {
				t.Errorf("dashboard UID %q is also used by %s", dashboard.UID, previous)
			} else {
				uids[dashboard.UID] = filepath.Base(path)
			}
			if dashboard.SchemaVersion != 42 {
				t.Errorf("schemaVersion = %d, want current Classic schema 42", dashboard.SchemaVersion)
			}
			if dashboard.Refresh != "30s" {
				t.Errorf("refresh = %q, want documented 30s scrape interval", dashboard.Refresh)
			}

			validateDashboardVariables(t, dashboard.Templating.List, contract)

			panelIDs := make(map[int]string)
			referenced := make(map[string]struct{})
			for i := range dashboard.Panels {
				validateDashboardPanel(t, &dashboard.Panels[i], contract, panelIDs, referenced)
			}
			expressions := strings.Join(dashboardExpressions(dashboard.Panels), "\n")
			if literalDeviceMACPattern.MatchString(expressions) {
				t.Error("distributable dashboard contains a hard-coded device MAC selector")
			}
			for _, selector := range []string{
				`device_status=~"^Connected.*$"`,
				`device_status!~"^Connected.*$"`,
			} {
				if !strings.Contains(expressions, selector) {
					t.Errorf("dashboard does not preserve wireless-backhaul status selector %s", selector)
				}
			}

			switch filepath.Base(path) {
			case "dashboard.json":
				if !strings.Contains(string(data), "/d/omada-device-details/omada-device-details?") ||
					!strings.Contains(string(data), "var-Device=${__field.labels.device_mac}") {
					t.Error("full dashboard does not link device uptime series to the device details dashboard")
				}
				required := []string{
					"omada_client_connected_total",
					"omada_collector_last_scrape_completed",
					"omada_controller_storage_total_bytes",
					"omada_device_6g_tx_util",
					"omada_device_temp",
					"omada_dpi_application_traffic_bytes",
					"omada_lag_link_rx",
					"omada_site_to_site_vpn_peer_status",
					"omada_wan_internet_state",
				}
				for _, metric := range required {
					if _, ok := referenced[metric]; !ok {
						t.Errorf("full dashboard no longer covers %s", metric)
					}
				}
			case "omada-device-details.json":
				required := []string{
					"omada_client_traffic_down_bytes",
					"omada_device_6g_tx_util",
					"omada_device_rx_rate",
					"omada_device_tx_rate",
					"omada_lag_link_speed_mbps",
					"omada_port_link_speed_mbps",
					"omada_wan_internet_state",
				}
				for _, metric := range required {
					if _, ok := referenced[metric]; !ok {
						t.Errorf("device details dashboard no longer covers %s", metric)
					}
				}
				for _, topologyLabel := range []string{"ap_mac", "switch_mac", "gateway_mac"} {
					if !strings.Contains(expressions, topologyLabel+`=~"${Device:regex}"`) {
						t.Errorf("device details dashboard does not scope clients by %s", topologyLabel)
					}
				}
			case "simple-omada-dashboard.json":
				if !strings.Contains(string(data), "/d/omada-device-details/omada-device-details?") ||
					!strings.Contains(string(data), "var-Device=${__field.labels.device_mac}") {
					t.Error("simple dashboard does not link device CPU series to the device details dashboard")
				}
			}
		})
	}
}

func validateDashboardVariables(t *testing.T, variables []grafanaVariable, contract map[string]map[string]struct{}) {
	t.Helper()
	if len(variables) < 4 {
		t.Fatalf("variable count = %d, want at least datasource, job, instance, and Site", len(variables))
	}

	datasource := variables[0]
	if datasource.Name != "datasource" || datasource.Type != "datasource" {
		t.Errorf("first variable = %s/%s, want datasource/datasource", datasource.Name, datasource.Type)
	}
	var datasourceQuery string
	if err := json.Unmarshal(datasource.Query, &datasourceQuery); err != nil || datasourceQuery != "prometheus" {
		t.Errorf("Datasource query = %s, want prometheus", datasource.Query)
	}
	for index, name := range []string{"datasource", "job", "instance", "Site"} {
		if variables[index].Name != name {
			t.Errorf("variable %d = %q, want %q", index, variables[index].Name, name)
		}
	}

	for _, variable := range variables {
		if variable.Type != "query" {
			continue
		}
		validatePrometheusDatasource(t, variable.Datasource, "variable "+variable.Name)
		if variable.Name == "Device" && !variable.Multi && !variable.IncludeAll {
			validateSingleDeviceVariable(t, variable, contract)
			continue
		}
		if !variable.Multi || !variable.IncludeAll {
			t.Errorf("variable %s must support multi-select and Include All", variable.Name)
		}
		wantAllValue := ".*"
		if variable.Name == "job" || variable.Name == "instance" {
			wantAllValue = ".+"
		}
		if variable.AllValue != wantAllValue {
			t.Errorf("variable %s allValue = %q, want %q", variable.Name, variable.AllValue, wantAllValue)
		}
		if variable.Refresh != 2 {
			t.Errorf("variable %s refresh = %d, want On Time Range Change (2)", variable.Name, variable.Refresh)
		}

		var query struct {
			Label   string
			Metric  string
			Query   string
			QryType int
		}
		if err := json.Unmarshal(variable.Query, &query); err != nil {
			t.Errorf("variable %s query is not a structured Prometheus query: %v", variable.Name, err)
			continue
		}
		if query.Metric == "" || query.Label == "" || query.Query == "" {
			t.Errorf("variable %s has incomplete Label values query", variable.Name)
			continue
		}
		if query.QryType != 1 {
			t.Errorf("variable %s qryType = %d, want Label values (1)", variable.Name, query.QryType)
		}

		metrics := validatePromQLContract(t, "variable "+variable.Name, query.Metric, "", contract)
		if len(metrics) != 1 {
			t.Errorf("variable %s metric query references %d metrics, want 1", variable.Name, len(metrics))
			continue
		}
		if _, ok := contract[metrics[0]][query.Label]; !ok && !isScrapeLabel(query.Label) {
			t.Errorf("variable %s label %q is not exported by %s", variable.Name, query.Label, metrics[0])
		}
	}
}

func validateSingleDeviceVariable(t *testing.T, variable grafanaVariable, contract map[string]map[string]struct{}) {
	t.Helper()
	if variable.Refresh != 2 {
		t.Errorf("variable Device refresh = %d, want On Time Range Change (2)", variable.Refresh)
	}
	for _, capture := range []string{"(?<value>", "(?<text>"} {
		if !strings.Contains(variable.Regex, capture) {
			t.Errorf("variable Device regex %q is missing %s capture", variable.Regex, capture)
		}
	}

	var query struct {
		Query   string
		QryType int
	}
	if err := json.Unmarshal(variable.Query, &query); err != nil {
		t.Errorf("variable Device query is not a structured Prometheus query: %v", err)
		return
	}
	if query.QryType != 3 || !strings.HasPrefix(query.Query, "query_result(") {
		t.Errorf("variable Device query = type %d %q, want Query result (3)", query.QryType, query.Query)
	}
	validateDashboardQueryScope(t, "variable Device", query.Query, contract)
	metrics := validatePromQLContract(t, "variable Device", query.Query, "", contract)
	if len(metrics) != 1 {
		t.Errorf("variable Device query references %d metrics, want 1", len(metrics))
		return
	}
	for _, label := range []string{"device_mac", "device_name"} {
		if _, ok := contract[metrics[0]][label]; !ok {
			t.Errorf("variable Device query metric %s does not export %s", metrics[0], label)
		}
	}
}

func validateDashboardPanel(t *testing.T, panel *grafanaPanel, contract map[string]map[string]struct{}, panelIDs map[int]string, referenced map[string]struct{}) {
	t.Helper()
	location := fmt.Sprintf("panel %d (%s)", panel.ID, panel.Title)

	if panel.ID <= 0 {
		t.Errorf("%s has invalid id", location)
	}
	if previous, exists := panelIDs[panel.ID]; exists {
		t.Errorf("%s duplicates id used by %s", location, previous)
	} else {
		panelIDs[panel.ID] = panel.Title
	}
	if strings.TrimSpace(panel.Title) == "" {
		t.Errorf("panel %d has an empty title", panel.ID)
	}
	if panel.GridPos.W < 1 || panel.GridPos.W > 24 || panel.GridPos.X < 0 || panel.GridPos.X+panel.GridPos.W > 24 || panel.GridPos.H < 1 || panel.GridPos.Y < 0 {
		t.Errorf("%s has invalid grid position %+v", location, panel.GridPos)
	}

	if panel.Type == "row" {
		for i := range panel.Panels {
			validateDashboardPanel(t, &panel.Panels[i], contract, panelIDs, referenced)
		}
		return
	}

	if strings.TrimSpace(panel.Description) == "" {
		t.Errorf("%s has an empty description", location)
	}
	validatePrometheusDatasource(t, panel.Datasource, location)
	if len(panel.Targets) == 0 {
		t.Errorf("%s has no query targets", location)
		return
	}
	validateDashboardPanelSemantics(t, panel, location)

	referenceIDs := make(map[string]struct{}, len(panel.Targets))
	for _, target := range panel.Targets {
		targetLocation := location + " target " + target.RefID
		validatePrometheusDatasource(t, target.Datasource, targetLocation)
		if target.RefID == "" {
			t.Errorf("%s has an empty refId", targetLocation)
		} else if _, exists := referenceIDs[target.RefID]; exists {
			t.Errorf("%s has a duplicate refId", targetLocation)
		} else {
			referenceIDs[target.RefID] = struct{}{}
		}
		if strings.TrimSpace(target.Expr) == "" {
			t.Errorf("%s has an empty expression", targetLocation)
			continue
		}
		validateDashboardQueryScope(t, targetLocation, target.Expr, contract)
		if target.Instant == target.Range {
			t.Errorf("%s must select exactly one of instant or range", targetLocation)
		}
		usesRate := strings.Contains(target.Expr, "rate(")
		if usesRate && target.Interval != dashboardRateMinStep {
			t.Errorf("%s rate query interval = %q, want Min step %q", targetLocation, target.Interval, dashboardRateMinStep)
		}
		if !usesRate && target.Interval != "" {
			t.Errorf("%s non-rate query has unexpected Min step %q", targetLocation, target.Interval)
		}
		for _, metric := range validatePromQLContract(t, targetLocation, target.Expr, target.LegendFormat, contract) {
			referenced[metric] = struct{}{}
		}
	}
}

func validateDashboardPanelSemantics(t *testing.T, panel *grafanaPanel, location string) {
	t.Helper()
	for _, target := range panel.Targets {
		if strings.Contains(target.Expr, "omada_wan_status{") {
			validateWANStatusMapping(t, panel.FieldConfig, location)
			break
		}
	}

	switch panel.Title {
	case "DPI-classified traffic", "DPI insight window":
		if len(panel.Targets) != 1 {
			t.Errorf("%s has %d targets, want 1", location, len(panel.Targets))
			return
		}
		if strings.Contains(panel.Targets[0].Expr, "vector(0)") {
			t.Errorf("%s converts unavailable DPI metrics to a misleading zero", location)
		}
	case "VPN uptime":
		if len(panel.Targets) != 1 {
			t.Errorf("%s has %d targets, want 1", location, len(panel.Targets))
			return
		}
		expression := panel.Targets[0].Expr
		for _, fragment := range []string{
			"omada_vpn_uptime",
			"omada_site_to_site_vpn_peer_login_timestamp",
			"clamp_min(time() - (",
			"> 0",
			"unless on (job, instance, site, site_id, name, vpn_type)",
			"max by (job, instance, site, site_id, vpn_id, name, vpn_type)",
		} {
			if !strings.Contains(expression, fragment) {
				t.Errorf("%s query does not contain required fallback fragment %q", location, fragment)
			}
		}
	}
}

func validateWANStatusMapping(t *testing.T, raw json.RawMessage, location string) {
	t.Helper()
	var fieldConfig struct {
		Defaults struct {
			Mappings []struct {
				Options map[string]struct {
					Color string
					Text  string
				}
			}
			Thresholds struct {
				Steps []struct {
					Color string
					Value *float64
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &fieldConfig); err != nil {
		t.Errorf("%s has invalid fieldConfig: %v", location, err)
		return
	}
	if len(fieldConfig.Defaults.Mappings) == 0 {
		t.Errorf("%s has no WAN status value mapping", location)
		return
	}
	options := fieldConfig.Defaults.Mappings[0].Options
	for value, want := range map[string]struct {
		text  string
		color string
	}{
		"0": {text: "Disconnected", color: "red"},
		"1": {text: "Connected", color: "green"},
	} {
		got, ok := options[value]
		if !ok || got.Text != want.text || got.Color != want.color {
			t.Errorf("%s WAN status %s mapping = %+v, want %s/%s", location, value, got, want.text, want.color)
		}
	}

	steps := fieldConfig.Defaults.Thresholds.Steps
	if len(steps) != 2 || steps[0].Value != nil || steps[0].Color != "red" ||
		steps[1].Value == nil || *steps[1].Value != 1 || steps[1].Color != "green" {
		t.Errorf("%s WAN status thresholds = %+v, want red below 1 and green from 1", location, steps)
	}
}

func validatePrometheusDatasource(t *testing.T, datasource *grafanaDatasource, location string) {
	t.Helper()
	if datasource == nil {
		t.Errorf("%s has no datasource", location)
		return
	}
	if datasource.Type != "prometheus" || datasource.UID != dashboardDatasourceUID {
		t.Errorf("%s datasource = %s/%s, want prometheus/%s", location, datasource.Type, datasource.UID, dashboardDatasourceUID)
	}
}

func validatePromQLContract(t *testing.T, location, expression, legend string, contract map[string]map[string]struct{}) []string {
	t.Helper()
	metricNames := uniqueStrings(metricNamePattern.FindAllString(expression, -1))
	if len(metricNames) == 0 {
		t.Errorf("%s references no omada metric", location)
		return nil
	}

	availableLabels := make(map[string]struct{})
	for label := range scrapeLabels {
		availableLabels[label] = struct{}{}
	}
	for _, metric := range metricNames {
		labels, ok := contract[metric]
		if !ok {
			t.Errorf("%s references unknown metric %s", location, metric)
			continue
		}
		for label := range labels {
			availableLabels[label] = struct{}{}
		}
	}

	for _, selector := range prometheusSelectors(expression) {
		labels, ok := contract[selector.metric]
		if !ok {
			continue
		}
		for _, match := range matcherPattern.FindAllStringSubmatch(selector.matchers, -1) {
			label := match[2]
			if _, ok := labels[label]; !ok && !isScrapeLabel(label) {
				t.Errorf("%s filters %s by unknown label %s", location, selector.metric, label)
			}
		}
	}

	for _, match := range groupByPattern.FindAllStringSubmatch(expression, -1) {
		for _, label := range commaSeparated(match[2]) {
			if _, ok := availableLabels[label]; !ok {
				t.Errorf("%s groups by label %s not exported by its metrics", location, label)
			}
		}
	}
	for _, match := range legendPattern.FindAllStringSubmatch(legend, -1) {
		if _, ok := availableLabels[match[1]]; !ok {
			t.Errorf("%s legend uses label %s not exported by its metrics", location, match[1])
		}
	}

	if strings.Contains(expression, "rate(") && !strings.Contains(expression, "$__rate_interval") {
		t.Errorf("%s uses rate() without $__rate_interval", location)
	}
	if fixedRangePattern.MatchString(expression) {
		t.Errorf("%s uses a fixed Prometheus range instead of $__rate_interval", location)
	}
	if exactVariableMatcherPattern.MatchString(expression) {
		t.Errorf("%s uses exact equality with a multi-value dashboard variable", location)
	}

	return metricNames
}

func validateDashboardQueryScope(t *testing.T, location, expression string, contract map[string]map[string]struct{}) {
	t.Helper()
	for _, selector := range prometheusSelectors(expression) {
		for _, matcher := range []string{`job=~"$job"`, `instance=~"$instance"`} {
			if !strings.Contains(selector.matchers, matcher) {
				t.Errorf("%s selector for %s is missing %s", location, selector.metric, matcher)
			}
		}
		if labels, ok := contract[selector.metric]; ok {
			if _, hasSite := labels["site"]; hasSite && !strings.Contains(selector.matchers, `site=~"${Site:regex}"`) {
				t.Errorf("%s selector for %s is missing the Site matcher", location, selector.metric)
			}
		}
	}
}

func dashboardExpressions(panels []grafanaPanel) []string {
	var expressions []string
	for _, panel := range panels {
		for _, target := range panel.Targets {
			expressions = append(expressions, target.Expr)
		}
		expressions = append(expressions, dashboardExpressions(panel.Panels)...)
	}
	return expressions
}

func isScrapeLabel(label string) bool {
	_, ok := scrapeLabels[label]
	return ok
}

func prometheusSelectors(expression string) []prometheusSelector {
	indexes := metricNamePattern.FindAllStringIndex(expression, -1)
	selectors := make([]prometheusSelector, 0, len(indexes))
	for _, index := range indexes {
		cursor := index[1]
		for cursor < len(expression) && (expression[cursor] == ' ' || expression[cursor] == '\t') {
			cursor++
		}
		if cursor >= len(expression) || expression[cursor] != '{' {
			continue
		}

		start := cursor + 1
		inQuote := false
		escaped := false
		for cursor = start; cursor < len(expression); cursor++ {
			character := expression[cursor]
			if escaped {
				escaped = false
				continue
			}
			if inQuote && character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inQuote = !inQuote
				continue
			}
			if character == '}' && !inQuote {
				selectors = append(selectors, prometheusSelector{
					metric:   expression[index[0]:index[1]],
					matchers: expression[start:cursor],
				})
				break
			}
		}
	}
	return selectors
}

func dashboardMetricContract(t *testing.T) map[string]map[string]struct{} {
	t.Helper()
	descriptors := make(chan *prometheus.Desc)
	go func() {
		for _, collector := range initCollectors(nil) {
			collector.Describe(descriptors)
		}
		newCollectorHealth().Describe(descriptors)
		close(descriptors)
	}()

	contract := make(map[string]map[string]struct{})
	for descriptor := range descriptors {
		name, _, rawLabels := parseDesc(descriptor.String())
		if !strings.HasPrefix(name, "omada_") {
			t.Fatalf("could not parse descriptor %s", descriptor)
		}
		labels := make(map[string]struct{})
		if rawLabels != "-" {
			for _, label := range commaSeparated(rawLabels) {
				labels[label] = struct{}{}
			}
		}
		contract[name] = labels
	}
	return contract
}

func commaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
