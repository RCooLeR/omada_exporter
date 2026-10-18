package omada

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

func TestClientDescriptorOnlyConstruction(t *testing.T) {
	client := NewClient(nil)
	if client != nil || client.Settings() != nil || !client.OpenAPIEnabled() {
		t.Fatal("nil session must preserve descriptor-only construction")
	}
	if controllerID, siteID := client.ContextIDs(); controllerID != "" || siteID != "" {
		t.Fatalf("unconfigured context = (%q, %q), want empty identifiers", controllerID, siteID)
	}
}

func TestClientUsesCurrentSharedSessionState(t *testing.T) {
	conf := &config.Config{Site: "Default"}
	shared := &session.Client{Config: conf, OpenAPIAuthMode: config.OpenAPIAuthDisabled}
	shared.SetContextIDs("old-controller", "old-site")
	client := NewClient(shared)
	if client.Settings() != conf || client.OpenAPIEnabled() {
		t.Fatal("facade did not preserve shared settings and disabled authentication")
	}
	shared.SetContextIDs("new-controller", "new-site")
	if controllerID, siteID := client.ContextIDs(); controllerID != "new-controller" || siteID != "new-site" {
		t.Fatalf("refreshed context = (%q, %q), want current session identifiers", controllerID, siteID)
	}
	for _, mode := range []string{config.OpenAPIAuthClientCredentials, config.OpenAPIAuthWebSession, ""} {
		shared.OpenAPIAuthMode = mode
		if !client.OpenAPIEnabled() {
			t.Errorf("resolved auth mode %q unexpectedly disabled", mode)
		}
	}
}

func TestClientsShareAuthenticationAndFeatureCaches(t *testing.T) {
	var alertRequests, ispRequests, tokenRequests atomic.Int32
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
			tokenRequests.Add(1)
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"accessToken":"test-token","refreshToken":"test-refresh","expiresIn":3600}}`)
		case "/cid/api/v2/sites/alert-count":
			alertRequests.Add(1)
			_, _ = io.WriteString(w, `{"errorCode":0,"result":[{"alertNum":3}]}`)
		case "/openapi/v1/cid/sites/site-id/dashboard/gateway/isp/load":
			ispRequests.Add(1)
			if req.Header.Get("Authorization") != "AccessToken=test-token" {
				t.Error("feature request did not use the shared session token")
			}
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"data":[{"name":"Gateway","ispInfo":{"ispArr":[{"name":"WAN"}]}}]}}`)
		default:
			t.Errorf("unexpected request path %s", req.URL.Path)
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)
	shared, err := session.Configure(&config.Config{
		Host: server.URL, Site: "Default", SystemType: config.SystemTypeStandard,
		OpenAPIAuth: config.OpenAPIAuthClientCredentials, ClientId: "test-client", SecretId: "test-secret",
		Timeout: 5, CacheTTL: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, second := NewClient(shared), NewClient(shared)
	var workers sync.WaitGroup
	for range 4 {
		for _, client := range []*Client{first, second} {
			workers.Go(func() {
				alert, err := client.GetAlert()
				if err != nil || alert == nil || alert.AlertNum != 3 {
					t.Errorf("GetAlert() = %v, %v; want alert count 3", alert, err)
				}
				isps, err := client.GetIsp()
				if err != nil || len(isps) != 1 || isps[0].GatewayName != "Gateway" {
					t.Errorf("GetIsp() = %v, %v; want enriched ISP entry", isps, err)
				}
			})
		}
	}
	workers.Wait()
	if alertRequests.Load() != 1 || ispRequests.Load() != 1 || tokenRequests.Load() != 1 {
		t.Fatalf("requests: alerts=%d ISP=%d tokens=%d; want one shared fetch per feature and one login", alertRequests.Load(), ispRequests.Load(), tokenRequests.Load())
	}
}
