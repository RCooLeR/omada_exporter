package omada

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

func TestNetworkClientsHTTPFallback(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		body           string
		v1Status       int
		reauthNotFound bool
		wantClients    int
		wantV1         int32
		wantError      string
	}{
		{name: "Spring 404", status: 404, body: `{"status":404,"error":"Not Found"}`, wantClients: 2, wantV1: 2},
		{name: "HTML 404", status: 404, body: `<html>Not Found</html>`, wantClients: 2, wantV1: 2},
		{name: "empty 404", status: 404, wantClients: 2, wantV1: 2},
		{name: "unsupported API", status: 200, body: `{"errorCode":-1600,"msg":"Unsupported request path"}`, wantClients: 2, wantV1: 2},
		{name: "v2 success", status: 200, body: `{"errorCode":0,"result":{"totalRows":1,"data":[{"mac":"v2-client"}]}}`, wantClients: 1},
		{name: "v2 empty", status: 200, body: `{"errorCode":0,"result":{"totalRows":0,"data":[]}}`},
		{name: "unauthorized", status: 401, wantError: "unauthorized after re-authentication"},
		{name: "forbidden", status: 403, wantError: "unauthorized after re-authentication"},
		{name: "rate limited", status: 429, wantError: "clients returned HTTP 429"},
		{name: "server failure", status: 500, wantError: "clients returned HTTP 500"},
		{name: "other API failure", status: 200, body: `{"errorCode":-1000,"msg":"Request failed"}`, wantError: "errorCode -1000"},
		{name: "malformed JSON", status: 200, body: `not JSON`, wantError: "invalid character"},
		{name: "v1 failure", status: 404, v1Status: 503, wantV1: 1, wantError: "clients v1 returned HTTP 503"},
		{name: "login endpoint 404", status: 401, reauthNotFound: true, wantError: "login returned HTTP 404"},
	}
	for _, systemType := range []string{config.SystemTypeStandard, config.SystemTypeFusion} {
		for _, tt := range tests {
			t.Run(systemType+"/"+tt.name, func(t *testing.T) {
				var v1Requests, v2Requests, authRequests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if strings.HasPrefix(req.URL.Path, "/openapi/v") {
						if systemType == config.SystemTypeFusion {
							if req.Header.Get("Csrf-Token") != "test-web-token" || req.Header.Get("Omada-Request-Source") != "web-local" || req.Header.Get("Authorization") != "" {
								t.Error("Fusion client request did not preserve web-session authentication")
							}
						} else if req.Header.Get("Authorization") != "AccessToken=test-token" {
							t.Error("standard client request did not preserve OpenAPI authentication")
						}
					}
					switch req.URL.Path {
					case "/api/info":
						_, _ = io.WriteString(w, `{"errorCode":0,"result":{"omadacId":"cid"}}`)
					case "/cid/api/v2/loginStatus":
						_, _ = fmt.Fprintf(w, `{"errorCode":0,"result":{"login":%t}}`, req.Header.Get("Csrf-Token") == "test-web-token")
					case "/cid/api/v2/login":
						if systemType == config.SystemTypeFusion && authRequests.Add(1) > 1 && tt.reauthNotFound {
							http.NotFound(w, req)
							return
						}
						_, _ = io.WriteString(w, `{"errorCode":0,"result":{"token":"test-web-token"}}`)
					case "/cid/api/v2/users/current":
						_, _ = io.WriteString(w, `{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`)
					case "/openapi/authorize/token":
						if systemType == config.SystemTypeFusion {
							t.Error("Fusion unexpectedly requested an OpenAPI token")
						}
						if authRequests.Add(1) > 1 && tt.reauthNotFound {
							http.NotFound(w, req)
							return
						}
						_, _ = io.WriteString(w, `{"errorCode":0,"result":{"accessToken":"test-token","refreshToken":"test-refresh","expiresIn":3600}}`)
					case "/openapi/v2/cid/sites/site-id/clients":
						v2Requests.Add(1)
						var body clientRequest
						if err := json.NewDecoder(req.Body).Decode(&body); err != nil || req.Method != http.MethodPost || body.Page != 1 || !body.Filters.Active {
							t.Errorf("invalid v2 client request: method=%s body=%+v error=%v", req.Method, body, err)
						}
						w.WriteHeader(tt.status)
						_, _ = io.WriteString(w, tt.body)
					case "/openapi/v1/cid/sites/site-id/clients":
						v1Requests.Add(1)
						if req.Method != http.MethodGet || req.URL.Query().Get("pageSize") != "100" {
							t.Errorf("invalid v1 client request: %s %s", req.Method, req.URL)
						}
						if tt.v1Status != 0 {
							w.WriteHeader(tt.v1Status)
							return
						}
						// Deliberately cap pages below the requested size to verify totalRows pagination.
						switch req.URL.Query().Get("page") {
						case "1":
							_, _ = io.WriteString(w, `{"errorCode":0,"result":{"totalRows":2,"data":[{"mac":"first","connectType":1,"wifiMode":3,"wireless":true,"apMac":"ap","trafficDown":42}]}}`)
						case "2":
							_, _ = io.WriteString(w, `{"errorCode":0,"result":{"totalRows":2,"data":[{"mac":"second","connect_type":1,"wifi_mode":3,"wireless":true,"apMac":"ap","trafficDown":42}]}}`)
						default:
							t.Errorf("unexpected v1 page: %s", req.URL.Query().Get("page"))
							http.Error(w, "unexpected page", http.StatusBadRequest)
						}
					default:
						t.Errorf("unexpected request path %s", req.URL.Path)
						http.NotFound(w, req)
					}
				}))
				t.Cleanup(server.Close)
				cfg := &config.Config{
					Host: server.URL, Site: "Default", SystemType: systemType,
					Username: "test-user", Password: "test-password", OpenAPIAuth: config.OpenAPIAuthAuto,
					Timeout: 5, CacheTTL: 60,
				}
				if systemType == config.SystemTypeStandard {
					cfg.ClientId, cfg.SecretId = "test-client", "test-secret"
				}
				shared, err := session.Configure(cfg)
				if err != nil {
					t.Fatal(err)
				}
				client := NewClient(shared)
				clients, err := client.GetNetworkClients()
				if tt.wantError != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantError) || len(clients) != 0 {
						t.Fatalf("GetNetworkClients() = %v, %v; want no clients and error containing %q", clients, err, tt.wantError)
					}
				} else {
					if err != nil || len(clients) != tt.wantClients {
						t.Fatalf("GetNetworkClients() = %v, %v; want %d clients", clients, err, tt.wantClients)
					}
					if tt.wantV1 == 2 {
						for i, mac := range []string{"first", "second"} {
							entry := clients[i]
							if entry.Mac != mac || entry.ConnectType != 1 || entry.WifiMode != 3 || !entry.Wireless || entry.ApMac != "ap" || entry.TrafficDown != 42 {
								t.Errorf("v1 page %d fields not preserved: %+v", i+1, entry)
							}
						}
					}
					beforeV1, beforeV2 := v1Requests.Load(), v2Requests.Load()
					cached, err := client.GetNetworkClients()
					if err != nil || len(cached) != len(clients) || v1Requests.Load() != beforeV1 || v2Requests.Load() != beforeV2 {
						t.Fatalf("second inventory request did not use the shared cache: %v, %v", cached, err)
					}
				}
				if v1Requests.Load() != tt.wantV1 {
					t.Errorf("v1 requests = %d, want %d", v1Requests.Load(), tt.wantV1)
				}
				if tt.reauthNotFound && authRequests.Load() != 2 {
					t.Errorf("authentication requests = %d, want initial login and one reauthentication attempt", authRequests.Load())
				}
			})
		}
	}
}
