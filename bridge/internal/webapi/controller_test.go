package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestGetControllerReturnsStatusWhenUpgradeChannelFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/info":
			_, _ = w.Write([]byte(`{"errorCode":0,"result":{"omadacId":"cid"}}`))
		case "/cid/api/v2/loginStatus":
			_, _ = w.Write([]byte(`{"errorCode":0,"result":{"login":true}}`))
		case "/cid/api/v2/users/current":
			_, _ = w.Write([]byte(`{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`))
		case "/cid/api/v2/settings/system/status":
			_, _ = w.Write([]byte(`{
				"errorCode": 0,
				"result": {
					"name": "Controller",
					"controllerVersion": "5.15.8",
					"model": "OC200",
					"deviceCapacity": {
						"apCapacity": 10,
						"oswCapacity": 10,
						"osgCapacity": 1,
						"oltCapacity": 1
					}
				}
			}`))
		case "/cid/api/v2/maintenance/software/channelUpdate":
			_, _ = w.Write([]byte(`{"errorCode":-1,"msg":"unable to reach update channel"}`))
		default:
			t.Fatalf("unexpected request path %s", req.URL.Path)
		}
	}))
	defer server.Close()

	apiClient, err := api.Configure(&config.Config{
		Host:       server.URL,
		Username:   "user",
		Password:   "pass",
		Site:       "Default",
		SystemType: config.SystemTypeStandard,
		CacheTTL:   0,
	})
	if err != nil {
		t.Fatalf("Configure() returned error: %v", err)
	}

	controller, err := (&Client{Client: apiClient}).GetController()
	if err != nil {
		t.Fatalf("GetController() returned error: %v", err)
	}
	if controller.Name != "Controller" {
		t.Fatalf("controller name = %q, want Controller", controller.Name)
	}
	if len(controller.UpgradeList) != 0 {
		t.Fatalf("upgrade list length = %d, want 0", len(controller.UpgradeList))
	}
}
