package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestGetAlertEncodesSiteIDAsJSON(t *testing.T) {
	const siteID = "site-\"quoted\\value"
	requestSiteIDs := make(chan []string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/info":
			_, _ = w.Write([]byte(`{"errorCode":0,"result":{"omadacId":"cid"}}`))
		case "/cid/api/v2/loginStatus":
			_, _ = w.Write([]byte(`{"errorCode":0,"result":{"login":true}}`))
		case "/cid/api/v2/users/current":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errorCode": 0,
				"result": map[string]any{"privilege": map[string]any{"sites": []map[string]string{{
					"name": "Default",
					"key":  siteID,
				}}}},
			})
		case "/cid/api/v2/sites/alert-count":
			var body struct {
				SiteIDs []string `json:"siteIds"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			requestSiteIDs <- body.SiteIDs
			_, _ = w.Write([]byte(`{"errorCode":0,"result":[{}]}`))
		default:
			http.Error(w, "unexpected request path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	apiClient, err := api.Configure(&config.Config{
		Host:       server.URL,
		Username:   "user",
		Password:   "pass",
		Site:       "Default",
		SystemType: config.SystemTypeStandard,
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	alert, err := (&Client{Client: apiClient}).GetAlert()
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if alert == nil {
		t.Fatal("GetAlert() returned nil alert")
	}
	got := <-requestSiteIDs
	if len(got) != 1 || got[0] != siteID {
		t.Fatalf("siteIds = %q, want [%q]", got, siteID)
	}
}
