package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestDisabledOpenAPIGathersRemainQuietAndPreserveWebMetrics(t *testing.T) {
	for _, auth := range []string{config.OpenAPIAuthAuto, config.OpenAPIAuthDisabled} {
		t.Run(auth, func(t *testing.T) {
			var output bytes.Buffer
			previousLogger, previousLevel := log.Logger, zerolog.GlobalLevel()
			log.Logger = zerolog.New(zerolog.SyncWriter(&output))
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)
			t.Cleanup(func() { log.Logger = previousLogger; zerolog.SetGlobalLevel(previousLevel) })

			var openAPIRequests, deviceRequests, portRequests, dpiRequests atomic.Int32
			var failDevices atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var body string
				switch r.URL.Path {
				case "/api/info":
					body = `{"errorCode":0,"result":{"omadacId":"cid"}}`
				case "/cid/api/v2/loginStatus":
					body = `{"errorCode":0,"result":{"login":true}}`
				case "/cid/api/v2/users/current":
					body = `{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Example","key":"site"}]}}}`
				case "/cid/api/v2/settings/system/status":
					body = `{"errorCode":0,"result":{"name":"Controller","upTime":60000}}`
				case "/cid/api/v2/maintenance/software/channelUpdate":
					body = `{"errorCode":0,"result":{"upgradeList":[]}}`
				case "/cid/api/v2/sites/alert-count":
					body = `{"errorCode":0,"result":[{"alertNum":0}]}`
				case "/cid/api/v2/sites/site/devices":
					deviceRequests.Add(1)
					if failDevices.Load() {
						w.WriteHeader(http.StatusServiceUnavailable)
						body = `{"errorCode":1,"msg":"temporary inventory failure"}`
					} else {
						body = `{"errorCode":0,"result":[{"type":"gateway","mac":"02-00-00-00-00-01","name":"Gateway","status":14},{"type":"ap","mac":"02-00-00-00-00-02","name":"AP","status":14}]}`
					}
				case "/cid/api/v2/sites/site/gateways/02-00-00-00-00-01":
					portRequests.Add(1)
					body = `{"errorCode":0,"result":{"temp":42,"rxRate":5,"txRate":7,"portStats":[{"port":1,"name":"LAN","status":1,"speed":3}]}}`
				case "/cid/api/v2/sites/site/stat/dpi/overview":
					dpiRequests.Add(1)
					body = `{"errorCode":0,"result":{"totalTraffic":64}}`
				case "/cid/api/v2/sites/site/stat/dpi/category/categoryCards":
					body = `{"errorCode":0,"result":{"categoryCards":[]}}`
				default:
					if strings.HasPrefix(r.URL.Path, "/openapi/") {
						openAPIRequests.Add(1)
					} else {
						t.Errorf("unexpected request %s", r.URL.Path)
					}
					w.WriteHeader(http.StatusNotFound)
					body = `{"errorCode":1,"msg":"unexpected request"}`
				}
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()

			client, err := api.Configure(&config.Config{
				Host: server.URL, Site: "Example", SystemType: config.SystemTypeStandard,
				OpenAPIAuth: auth, Timeout: 2, CacheTTL: 0,
				TrackPortMetrics: true, TrackInsightMetrics: true, TrackNetworkQualityMetrics: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if client.OpenAPIAuthMode != config.OpenAPIAuthDisabled {
				t.Fatalf("resolved mode=%q, want disabled", client.OpenAPIAuthMode)
			}
			collectors := initCollectors(client)
			// HTTP and MQTT receive the same collectors; exercise both registries repeatedly.
			registries := []*prometheus.Registry{prometheus.NewRegistry(), prometheus.NewRegistry()}
			for _, registry := range registries {
				for _, collector := range collectors {
					registry.MustRegister(collector)
				}
			}
			for range 2 {
				for _, registry := range registries {
					families, err := registry.Gather()
					if err != nil {
						t.Fatal(err)
					}
					for _, name := range []string{"omada_controller_uptime_seconds", "omada_site_alert_num", "omada_device_temp", "omada_port_link_status", "omada_dpi_total_traffic_bytes"} {
						if !hasMetricFamily(families, name) {
							t.Errorf("Web API metric %q missing", name)
						}
					}
					for _, family := range families {
						for _, prefix := range []string{"omada_client_", "omada_vpn_", "omada_isp_", "omada_wan_", "omada_network_quality_", "omada_ap_uplink_", "omada_gateway_wan_quality_"} {
							if strings.HasPrefix(family.GetName(), prefix) {
								t.Errorf("disabled telemetry %q was emitted", family.GetName())
							}
						}
					}
				}
			}
			if openAPIRequests.Load() != 0 || deviceRequests.Load() != 4 || portRequests.Load() != 4 || dpiRequests.Load() != 4 {
				t.Errorf("requests OpenAPI/devices/ports/DPI=%d/%d/%d/%d, want 0/4/4/4", openAPIRequests.Load(), deviceRequests.Load(), portRequests.Load(), dpiRequests.Load())
			}
			wantNotices := 0
			if auth == config.OpenAPIAuthAuto {
				wantNotices = 1
			}
			if got := strings.Count(output.String(), `"event":"openapi_disabled"`); got != wantNotices {
				t.Errorf("startup notices=%d, want %d; logs=%s", got, wantNotices, &output)
			}
			if strings.Contains(output.String(), `"level":"error"`) {
				t.Fatalf("expected polling failure logged: %s", &output)
			}
			output.Reset()
			failDevices.Store(true)
			if _, err := registries[0].Gather(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), `"level":"error"`) || !strings.Contains(output.String(), "temporary inventory failure") {
				t.Errorf("real Web API failure was hidden: %s", &output)
			}
		})
	}
}
