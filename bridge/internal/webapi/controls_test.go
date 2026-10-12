package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
)

func newControlsTestClient(t *testing.T, kind string, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/info":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"omadacId":"cid"}}`)
		case "/cid/api/v2/loginStatus":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"login":true}}`)
		case "/cid/api/v2/users/current":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`)
		default:
			handler(w, req)
		}
	}))
	t.Cleanup(server.Close)
	client, err := api.Configure(&config.Config{Host: server.URL, Site: "Default", SystemType: kind, OpenAPIAuth: config.OpenAPIAuthDisabled, CacheTTL: 3600, Timeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	return &Client{Client: client}
}

func TestControlActionsUseVerifiedWebRoutesForBothControllerKinds(t *testing.T) {
	for _, kind := range []string{config.SystemTypeStandard, config.SystemTypeFusion} {
		for _, action := range []string{ControlActionRebootDevice, ControlActionReconnectWiFiClient} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				var reads, posts atomic.Int32
				client := newControlsTestClient(t, kind, func(w http.ResponseWriter, req *http.Request) {
					switch req.URL.Path {
					case "/cid/api/v2/sites/site-id/devices":
						reads.Add(1)
						_, _ = io.WriteString(w, `{"errorCode":0,"result":[{"mac":"aa:bb:cc:dd:ee:ff","type":"ap","status":15,"name":"Mesh AP"}]}`)
					case "/cid/api/v2/sites/site-id/clients":
						reads.Add(1)
						if req.URL.Query().Get("filters.active") != "true" {
							t.Error("missing active filter")
						}
						_, _ = io.WriteString(w, `{"errorCode":0,"result":{"totalRows":1,"data":[{"mac":"AA-BB-CC-DD-EE-FF","wireless":true,"active":true,"authStatus":0}]}}`)
					case "/cid/api/v2/sites/site-id/cmd/devices/AA-BB-CC-DD-EE-FF/reboot", "/cid/api/v2/sites/site-id/cmd/clients/AA-BB-CC-DD-EE-FF/reconnect":
						posts.Add(1)
						if req.Method != http.MethodPost {
							t.Errorf("method = %s", req.Method)
						}
						body, _ := io.ReadAll(req.Body)
						if action == ControlActionRebootDevice && string(body) != `{"mac":"AA-BB-CC-DD-EE-FF"}` {
							t.Errorf("reboot body = %s", body)
						}
						if action == ControlActionReconnectWiFiClient && len(body) != 0 {
							t.Errorf("reconnect body = %s, want empty", body)
						}
						_, _ = io.WriteString(w, `{"errorCode":0,"result":{}}`)
					default:
						t.Errorf("unexpected endpoint %s", req.URL)
						http.Error(w, "unexpected", 500)
					}
				})
				for range 2 {
					var err error
					if action == ControlActionRebootDevice {
						err = client.RebootDevice(t.Context(), "site-id", "aabbccddeeff")
					} else {
						err = client.ReconnectWiFiClient(t.Context(), "site-id", "aa:bb:cc:dd:ee:ff")
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if reads.Load() != 2 || posts.Load() != 2 {
					t.Fatalf("reads=%d posts=%d; expected fresh validation on every command", reads.Load(), posts.Load())
				}
			})
		}
	}
}

func TestControlsRejectIneligibleAndForeignTargets(t *testing.T) {
	for _, tc := range []struct{ name, action, row string }{
		{"offline", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"ap","status":0}`},
		{"rebooting", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"ap","status":13}`},
		{"upgrading", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"ap","status":12}`},
		{"migrating", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"ap","status":16}`},
		{"controller", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"controller","status":14}`},
		{"olt", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"olt","status":14}`},
		{"stack-member", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"switch","status":14,"stackMsg":{"stackId":"stack"}}`},
		{"foreign-device", ControlActionRebootDevice, `{"mac":"AA-BB-CC-DD-EE-FF","type":"gateway","status":14,"siteId":"other-site"}`},
		{"wired", ControlActionReconnectWiFiClient, `{"mac":"AA-BB-CC-DD-EE-FF","wireless":false,"active":true,"authStatus":0}`},
		{"inactive", ControlActionReconnectWiFiClient, `{"mac":"AA-BB-CC-DD-EE-FF","wireless":true,"active":false,"authStatus":0}`},
		{"blocked", ControlActionReconnectWiFiClient, `{"mac":"AA-BB-CC-DD-EE-FF","wireless":true,"active":true,"authStatus":5}`},
		{"disconnected", ControlActionReconnectWiFiClient, `{"mac":"AA-BB-CC-DD-EE-FF","wireless":true,"active":true,"authStatus":4}`},
		{"missing-state", ControlActionReconnectWiFiClient, `{"mac":"AA-BB-CC-DD-EE-FF","wireless":true,"active":true}`},
		{"foreign-client", ControlActionReconnectWiFiClient, `{"mac":"AA-BB-CC-DD-EE-FF","wireless":true,"active":true,"authStatus":0,"siteId":"other-site"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts atomic.Int32
			client := newControlsTestClient(t, config.SystemTypeFusion, func(w http.ResponseWriter, req *http.Request) {
				if req.Method == http.MethodPost {
					posts.Add(1)
				}
				if strings.HasSuffix(req.URL.Path, "/devices") {
					_, _ = fmt.Fprintf(w, `{"errorCode":0,"result":[%s]}`, tc.row)
				} else {
					_, _ = fmt.Fprintf(w, `{"errorCode":0,"result":{"totalRows":1,"data":[%s]}}`, tc.row)
				}
			})
			if err := client.control(t.Context(), "site-id", "aabbccddeeff", tc.action); err == nil {
				t.Fatal("ineligible target accepted")
			}
			if posts.Load() != 0 {
				t.Fatal("ineligible target received a command")
			}
		})
	}
}

func TestControlInputValidationDoesNotMakeInventoryOrActionRequests(t *testing.T) {
	client := newControlsTestClient(t, config.SystemTypeStandard, func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("unexpected request: %s", req.URL)
		http.Error(w, "unexpected", 500)
	})
	for _, mac := range []string{"", "../reboot", "AA-BB-CC-DD-EE", "aa:bb-cc:dd:ee:ff", " aa:bb:cc:dd:ee:ff", "aabbccddeeff?site=other", "aabbccddeeff00"} {
		if err := client.RebootDevice(t.Context(), "site-id", mac); err == nil {
			t.Errorf("MAC %q accepted", mac)
		}
	}
	for _, site := range []string{"", "other-site"} {
		if err := client.ReconnectWiFiClient(t.Context(), site, "aabbccddeeff"); err == nil {
			t.Errorf("site %q accepted", site)
		}
	}
}

func TestControlTargetsPaginationAndMetadata(t *testing.T) {
	client := newControlsTestClient(t, config.SystemTypeStandard, func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/devices") {
			_, _ = io.WriteString(w, `{"errorCode":0,"result":[{"mac":"112233445566","type":"gateway","status":14,"name":"Fusion","model":"ER706W"}]}`)
			return
		}
		if req.URL.Query().Get("page") == "1" {
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"totalRows":101,"data":[{"mac":"aabbccddeeff","name":"Phone","wireless":true,"active":true,"authStatus":3}]}}`)
			return
		}
		if req.URL.Query().Get("page") != "2" {
			t.Errorf("unexpected page %s", req.URL.Query().Get("page"))
		}
		_, _ = io.WriteString(w, `{"errorCode":0,"result":{"totalRows":101,"data":[{"mac":"aabbccddee00","name":"Tablet","wireless":true,"active":true,"authStatus":2}]}}`)
	})
	targets, err := client.ControlTargets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Fatalf("targets = %+v", targets)
	}
	if targets[0].MAC != "11-22-33-44-55-66" || targets[0].Action != ControlActionRebootDevice || targets[0].Model != "ER706W" {
		t.Errorf("gateway target = %+v", targets[0])
	}
	for _, target := range targets {
		if target.SiteID != "site-id" {
			t.Errorf("wrong scope: %+v", target)
		}
	}
}

func TestControlsFailOnInventoryOrCommandErrorsWithoutReplay(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		inventory  bool
	}{
		{"inventory-api", `{"errorCode":-1,"msg":"permission denied"}`, 200, true},
		{"unauthorized", `{"errorCode":-1200,"msg":"login expired"}`, 200, false},
		{"forbidden", `{"errorCode":403}`, 403, false},
		{"unsupported", `{"errorCode":-1600}`, 200, false},
		{"missing-ack", `{}`, 200, false},
		{"html", `<html>login</html>`, 200, false},
		{"invalid-json", `{`, 200, false},
		{"server-error", `{"errorCode":-1}`, 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts atomic.Int32
			client := newControlsTestClient(t, config.SystemTypeStandard, func(w http.ResponseWriter, req *http.Request) {
				if req.Method == http.MethodPost {
					posts.Add(1)
				}
				if req.Method == http.MethodGet && !tc.inventory {
					_, _ = io.WriteString(w, `{"errorCode":0,"result":[{"mac":"aabbccddeeff","type":"switch","status":14}]}`)
					return
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			if err := client.RebootDevice(t.Context(), "site-id", "aabbccddeeff"); err == nil {
				t.Fatal("error response accepted")
			}
			want := int32(1)
			if tc.inventory {
				want = 0
			}
			if posts.Load() != want {
				t.Errorf("command attempts=%d, want%d", posts.Load(), want)
			}
		})
	}
}

func TestCanonicalControlMAC(t *testing.T) {
	for _, value := range []string{"aabbccddeeff", "aa:bb:cc:dd:ee:ff", "AA-BB-CC-DD-EE-FF"} {
		if got, err := CanonicalControlMAC(value); err != nil || got != "AA-BB-CC-DD-EE-FF" {
			t.Errorf("CanonicalControlMAC(%q)=%q,%v", value, got, err)
		}
	}
}

func TestControlInventoryDuplicateTargetsRejected(t *testing.T) {
	for _, action := range []string{ControlActionRebootDevice, ControlActionReconnectWiFiClient} {
		t.Run(action, func(t *testing.T) {
			client := newControlsTestClient(t, config.SystemTypeStandard, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet {
					t.Error("duplicate inventory caused command")
				}
				row := map[string]any{"mac": "aabbccddeeff", "type": "ap", "status": 14, "active": true, "wireless": true, "authStatus": 0}
				result := any([]any{row, row})
				if action == ControlActionReconnectWiFiClient {
					result = map[string]any{"totalRows": 2, "data": []any{row, row}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			})
			if err := client.control(context.Background(), "site-id", "aabbccddeeff", action); err == nil {
				t.Fatal("ambiguous duplicate accepted")
			}
		})
	}
}
