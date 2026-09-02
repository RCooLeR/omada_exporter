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

const dashboardDatasourceUID = "$datasource"

var scrapeLabels = map[string]struct{}{"instance": {}, "job": {}}

var (
	metricNamePattern           = regexp.MustCompile("\\bomada_[a-zA-Z0-9_]+\\b")
	matcherPattern              = regexp.MustCompile("(^|,)\\s*([a-zA-Z_][a-zA-Z0-9_]*)\\s*(=~|!~|!=|=)")
	groupByPattern              = regexp.MustCompile("\\b(sum|avg|max|min|count)\\s+by\\s*\\(([^)]*)\\)")
	legendPattern               = regexp.MustCompile("\\{\\{\\s*([a-zA-Z_][a-zA-Z0-9_]*)\\s*\\}\\}")
	fixedRangePattern           = regexp.MustCompile("\\[[0-9]+[smhdwy]\\]")
	exactVariableMatcherPattern = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*\\s*=\\s*"\\$(?:\\{[^}]+\\}|[a-zA-Z_][a-zA-Z0-9_]*)"`)
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
	Type       string
}

type grafanaPanel struct {
	Datasource  *grafanaDatasource
	Description string
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
	if len(paths) < 2 {
		t.Fatalf("dashboard count = %d, want at least 2", len(paths))
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
			for _, selector := range []string{
				`device_status=~"^Connected.*$"`,
				`device_status!~"^Connected.*$"`,
			} {
				if !strings.Contains(expressions, selector) {
					t.Errorf("dashboard does not preserve wireless-backhaul status selector %s", selector)
				}
			}

			if filepath.Base(path) == "dashboard.json" {
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
		for _, metric := range validatePromQLContract(t, targetLocation, target.Expr, target.LegendFormat, contract) {
			referenced[metric] = struct{}{}
		}
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
