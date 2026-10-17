package webapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestGetDevicesSkipsOnlyUnavailableOpenAPIEnrichment(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       string
		wanStatus  int
		legacyMode bool
	}{
		{name: "disabled", mode: config.OpenAPIAuthDisabled, wanStatus: http.StatusOK},
		{name: "web session", mode: config.OpenAPIAuthWebSession, wanStatus: http.StatusOK},
		{name: "client credentials", mode: config.OpenAPIAuthClientCredentials, wanStatus: http.StatusOK},
		{name: "legacy empty mode", mode: config.OpenAPIAuthClientCredentials, wanStatus: http.StatusOK, legacyMode: true},
		{name: "enabled request failure", mode: config.OpenAPIAuthWebSession, wanStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			previousLogger, previousLevel := log.Logger, zerolog.GlobalLevel()
			log.Logger = zerolog.New(&logs)
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)
			t.Cleanup(func() {
				log.Logger = previousLogger
				zerolog.SetGlobalLevel(previousLevel)
			})

			var inventoryRequests, portRequests, wanRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/info":
					_, _ = io.WriteString(w, `{"errorCode":0,"result":{"omadacId":"cid"}}`)
				case "/cid/api/v2/loginStatus":
					_, _ = io.WriteString(w, `{"errorCode":0,"result":{"login":true}}`)
				case "/cid/api/v2/users/current":
					_, _ = io.WriteString(w, `{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`)
				case "/openapi/authorize/token":
					_, _ = io.WriteString(w, `{"errorCode":0,"result":{"accessToken":"test-access-token","refreshToken":"test-refresh-token","expiresIn":3600}}`)
				case "/cid/api/v2/sites/site-id/devices":
					inventoryRequests.Add(1)
					_, _ = io.WriteString(w, `{"errorCode":0,"result":[{"mac":"02-00-00-00-00-01","type":"gateway","name":"Test Gateway","status":14}]}`)
				case "/cid/api/v2/sites/site-id/gateways/02-00-00-00-00-01":
					portRequests.Add(1)
					_, _ = io.WriteString(w, `{"errorCode":0,"result":{"temp":42,"rxRate":123,"txRate":456,"portStats":[{"port":1,"name":"Test Port","status":1,"speed":3}]}}`)
				case "/openapi/v1/cid/sites/site-id/gateways/02-00-00-00-00-01/wan-status":
					wanRequests.Add(1)
					w.WriteHeader(tc.wanStatus)
					if tc.wanStatus != http.StatusOK {
						_, _ = io.WriteString(w, `{"errorCode":-1,"msg":"temporary WAN API failure"}`)
						return
					}
					_, _ = io.WriteString(w, `{"errorCode":0,"result":[{"port":1,"name":"Test WAN","status":1,"internetState":1}]}`)
				default:
					t.Errorf("unexpected request path %s", req.URL.Path)
					http.NotFound(w, req)
				}
			}))
			t.Cleanup(server.Close)
			conf := &config.Config{Host: server.URL, Site: "Default", SystemType: config.SystemTypeStandard, OpenAPIAuth: tc.mode, CacheTTL: 0, Timeout: 5}
			if tc.mode == config.OpenAPIAuthClientCredentials {
				conf.ClientId, conf.SecretId = "test-client", "test-secret"
			}
			apiClient, err := api.Configure(conf)
			if err != nil {
				t.Fatalf("Configure() error = %v", err)
			}
			if tc.legacyMode {
				apiClient.OpenAPIAuthMode = ""
			}
			client := &Client{Client: apiClient}
			wantWANCount := 0
			if tc.mode != config.OpenAPIAuthDisabled && tc.wanStatus == http.StatusOK {
				wantWANCount = 1
			}
			for range 2 {
				devices, err := client.GetDevices()
				if err != nil || len(devices) != 1 {
					t.Fatalf("GetDevices() = %v, %v; want one gateway", devices, err)
				}
				gateway, ok := devices[0].(*model.Gateway)
				if !ok {
					t.Fatalf("device type = %T, want *model.Gateway", devices[0])
				}
				if len(gateway.Ports) != 1 || gateway.Temp != 42 || gateway.RxRate != 123 || gateway.TxRate != 456 {
					t.Fatalf("Web API gateway enrichment was lost: %+v", gateway)
				}
				if len(gateway.Wans) != wantWANCount {
					t.Errorf("WAN count = %d, want %d", len(gateway.Wans), wantWANCount)
				}
			}
			if inventoryRequests.Load() != 2 || portRequests.Load() != 2 {
				t.Errorf("inventory/port requests = %d/%d, want 2/2", inventoryRequests.Load(), portRequests.Load())
			}
			wantWANRequests := int32(2)
			if tc.mode == config.OpenAPIAuthDisabled {
				wantWANRequests = 0
			}
			if got := wanRequests.Load(); got != wantWANRequests {
				t.Errorf("WAN requests = %d, want %d", got, wantWANRequests)
			}
			if tc.wanStatus == http.StatusOK {
				if logs.Len() != 0 {
					t.Errorf("unexpected error logs: %s", logs.String())
				}
			} else if got := strings.Count(logs.String(), "Error getting wans"); got != 2 {
				t.Errorf("WAN error logs = %d, want 2; logs: %s", got, logs.String())
			}
		})
	}
}

func TestEnrichDevicesRunsConcurrentlyWithinLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		devices := make([]model.DevicesInterface, deviceEnrichmentConcurrency*2)
		for i := range devices {
			devices[i] = &model.Switch{}
		}

		started := make(chan struct{}, len(devices))
		release := make(chan struct{})
		finished := make(chan struct{})
		var active atomic.Int32
		var maximum atomic.Int32
		go func() {
			enrichDevices(devices, func(model.DevicesInterface) {
				current := active.Add(1)
				for {
					previous := maximum.Load()
					if current <= previous || maximum.CompareAndSwap(previous, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				active.Add(-1)
			})
			close(finished)
		}()

		for range deviceEnrichmentConcurrency {
			<-started
		}
		synctest.Wait()
		select {
		case <-started:
			t.Fatalf("more than %d enrichments started concurrently", deviceEnrichmentConcurrency)
		default:
		}
		close(release)

		<-finished
		if got := maximum.Load(); got != deviceEnrichmentConcurrency {
			t.Fatalf("maximum concurrent enrichments = %d, want %d", got, deviceEnrichmentConcurrency)
		}
	})
}
