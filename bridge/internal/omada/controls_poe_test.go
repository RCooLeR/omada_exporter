package omada

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

const poeTestMAC = "AA-BB-CC-DD-EE-FF"

func poeTestDetail(kind string) map[string]any {
	detail := map[string]any{"mac": poeTestMAC, "siteId": "site-id", "site": "site-id", "siteName": "Default"}
	if kind == "switch" {
		detail["devCap"] = map[string]any{"poeSupport": true}
		detail["uplink"] = map[string]any{"port": 8}
	} else {
		detail["supportPoe"] = true
		detail["portConfigs"] = []map[string]any{poeTestGatewayPort()}
	}
	return detail
}

func poeTestSwitchPort() map[string]any {
	return map[string]any{"port": 3, "name": "Camera", "operation": "switching", "supportPoe": true,
		"portStatus": map[string]any{"port": 3, "poe": true}}
}

func poeTestGatewayPort() map[string]any {
	return map[string]any{"port": 3, "portSupportPoe": true,
		"portStat": map[string]any{"port": 3, "poe": true, "mode": 1, "name": "LAN3"}}
}

func writePoEResult(t *testing.T, w http.ResponseWriter, result any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result}); err != nil {
		t.Fatal(err)
	}
}

func writeEmptyPoEEndpointLists(t *testing.T, w http.ResponseWriter, req *http.Request) bool {
	if strings.Contains(req.URL.Path, "/eligible-") {
		writePoEResult(t, w, map[string]any{"currentPage": 1, "totalRows": 0, "data": []any{}})
		return true
	}
	return false
}

func TestPowerCyclePoENativeRoutesForBothControllerKinds(t *testing.T) {
	for _, kind := range []string{config.SystemTypeStandard, config.SystemTypeFusion} {
		for _, ownerType := range []string{"switch", "gateway"} {
			t.Run(kind+"/"+ownerType, func(t *testing.T) {
				var reads, posts atomic.Int32
				prefix := "/cid/api/v2/sites/site-id/"
				detailPath := prefix + ownerType + "es/" + poeTestMAC
				if ownerType == "gateway" {
					detailPath = prefix + "gateways/" + poeTestMAC
				}
				commandPath := detailPath + "/ports/3/restart"
				if ownerType == "switch" {
					commandPath = prefix + "cmd/switches/" + poeTestMAC + "/ports/3/restart"
				}
				client := newControlsTestClient(t, kind, func(w http.ResponseWriter, req *http.Request) {
					if writeEmptyPoEEndpointLists(t, w, req) {
						return
					}
					switch req.URL.Path {
					case prefix + "devices":
						reads.Add(1)
						writePoEResult(t, w, []map[string]any{{"mac": poeTestMAC, "name": "Power source", "type": ownerType, "status": 14}})
					case detailPath:
						writePoEResult(t, w, poeTestDetail(ownerType))
					case detailPath + "/ports":
						writePoEResult(t, w, []map[string]any{poeTestSwitchPort()})
					case commandPath:
						posts.Add(1)
						body, _ := io.ReadAll(req.Body)
						if req.Method != http.MethodPost || len(body) != 0 || req.URL.RawQuery != "" {
							t.Errorf("native restart must be one empty POST, got %s %q", req.Method, body)
						}
						writePoEResult(t, w, map[string]any{})
					default:
						t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
						http.Error(w, "unexpected", 500)
					}
				})
				targets, err := client.PoEControlTargets(t.Context())
				if err != nil || len(targets) != 1 {
					t.Fatalf("targets=%+v err=%v", targets, err)
				}
				if targets[0].Action != ControlActionPowerCyclePoE || targets[0].MAC != poeTestMAC || targets[0].Port != 3 || targets[0].SiteID != "site-id" || targets[0].PoweredDeviceMAC != "" {
					t.Fatalf("incorrect port target: %+v", targets[0])
				}
				for range 2 {
					if err := client.PowerCyclePoEPort(t.Context(), "site-id", "aa:bb:cc:dd:ee:ff", 3, ""); err != nil {
						t.Fatal(err)
					}
				}
				if reads.Load() != 3 || posts.Load() != 2 {
					t.Fatalf("inventory reads=%d posts=%d; every click must revalidate", reads.Load(), posts.Load())
				}
			})
		}
	}
}

func TestPoESwitchRejectsUnsafePortInventory(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(map[string]any, *([]map[string]any))
	}{
		{"missing-capability", func(d map[string]any, _ *([]map[string]any)) { delete(d, "devCap") }},
		{"wrong-owner", func(d map[string]any, _ *([]map[string]any)) { d["mac"] = "00-11-22-33-44-55" }},
		{"wrong-site", func(d map[string]any, _ *([]map[string]any)) { d["siteId"] = "foreign" }},
		{"wrong-site-alias", func(d map[string]any, _ *([]map[string]any)) { d["site"] = "foreign" }},
		{"uplink", func(d map[string]any, _ *([]map[string]any)) { d["uplink"] = map[string]any{"port": 3} }},
		{"stack-port", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["configStack"] = true }},
		{"mad-port", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["madUsed"] = true }},
		{"mlag-peer", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["configMlagPeerLink"] = true }},
		{"mlag-dad", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["configMlagDad"] = true }},
		{"lag", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["operation"] = "aggregating" }},
		{"unknown-operation", func(_ map[string]any, p *([]map[string]any)) { delete((*p)[0], "operation") }},
		{"disabled-poe", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["portStatus"].(map[string]any)["poe"] = false }},
		{"unsupported-poe", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["supportPoe"] = false }},
		{"status-port-mismatch", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["portStatus"].(map[string]any)["port"] = 4 }},
		{"missing-state", func(_ map[string]any, p *([]map[string]any)) { delete((*p)[0], "portStatus") }},
		{"duplicate-port", func(_ map[string]any, p *([]map[string]any)) { *p = append(*p, poeTestSwitchPort()) }},
		{"invalid-port", func(_ map[string]any, p *([]map[string]any)) { (*p)[0]["port"] = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail, ports := poeTestDetail("switch"), []map[string]any{poeTestSwitchPort()}
			test.edit(detail, &ports)
			client := newControlsTestClient(t, config.SystemTypeStandard, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet {
					t.Fatalf("unsafe mutation: %s", req.URL.Path)
				}
				switch {
				case strings.HasSuffix(req.URL.Path, "/devices"):
					writePoEResult(t, w, []map[string]any{{"mac": poeTestMAC, "type": "switch", "status": 14}})
				case strings.HasSuffix(req.URL.Path, "/ports"):
					writePoEResult(t, w, ports)
				default:
					writePoEResult(t, w, detail)
				}
			})
			if targets, err := client.PoEControlTargets(t.Context()); err != nil || len(targets) != 0 {
				t.Fatalf("unsafe discovery: %+v, %v", targets, err)
			}
			if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, 3, ""); err == nil {
				t.Fatal("unsafe PoE command accepted")
			}
		})
	}
}

func TestPoEGatewayRejectsUnsupportedOrUnsafePorts(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"missing-owner-capability", func(d, p map[string]any) { delete(d, "supportPoe") }},
		{"foreign-site", func(d, p map[string]any) { d["site"] = "foreign" }},
		{"missing-port-capability", func(d, p map[string]any) { delete(p, "portSupportPoe") }},
		{"unsupported-port", func(d, p map[string]any) { d["unsupportedPorts"] = []int{3} }},
		{"mirror", func(d, p map[string]any) { p["mirrorEnable"] = true }},
		{"wan", func(d, p map[string]any) { p["portStat"].(map[string]any)["mode"] = 0 }},
		{"missing-mode", func(d, p map[string]any) { delete(p["portStat"].(map[string]any), "mode") }},
		{"not-powered", func(d, p map[string]any) { p["portStat"].(map[string]any)["poe"] = false }},
		{"mismatched-port", func(d, p map[string]any) { p["portStat"].(map[string]any)["port"] = 4 }},
		{"duplicate", func(d, p map[string]any) { d["portConfigs"] = []map[string]any{p, p} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail := poeTestDetail("gateway")
			port := detail["portConfigs"].([]map[string]any)[0]
			test.edit(detail, port)
			client := newControlsTestClient(t, config.SystemTypeFusion, func(w http.ResponseWriter, req *http.Request) {
				if writeEmptyPoEEndpointLists(t, w, req) {
					return
				}
				if req.Method != http.MethodGet {
					t.Fatalf("unsafe mutation: %s", req.URL.Path)
				}
				if strings.HasSuffix(req.URL.Path, "/devices") {
					writePoEResult(t, w, []map[string]any{{"mac": poeTestMAC, "type": "gateway", "status": 14}})
				} else {
					writePoEResult(t, w, detail)
				}
			})
			if targets, err := client.PoEControlTargets(t.Context()); err != nil || len(targets) != 0 {
				t.Fatalf("unsafe discovery: %+v, %v", targets, err)
			}
			if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, 3, ""); err == nil {
				t.Fatal("unsafe PoE command accepted")
			}
		})
	}
}

func TestPoEInputValidationBeforeInventory(t *testing.T) {
	client := newControlsTestClient(t, config.SystemTypeStandard, func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("unexpected inventory or command: %s", req.URL.Path)
		http.Error(w, "unexpected", 500)
	})
	for _, port := range []int{-1, 0, 256} {
		if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, port, ""); err == nil {
			t.Errorf("accepted port %d", port)
		}
	}
	for _, test := range []struct{ site, mac, pd string }{
		{"foreign-site", poeTestMAC, ""}, {"", poeTestMAC, ""}, {"site-id", "../bad", ""}, {"site-id", poeTestMAC, "invalid"},
	} {
		if err := client.PowerCyclePoEPort(t.Context(), test.site, test.mac, 3, test.pd); err == nil {
			t.Fatal("invalid PoE target accepted")
		}
	}
}

func TestPoERevalidatesStateAndNeverRetriesCommand(t *testing.T) {
	for _, failure := range []string{"poe-disabled-after-discovery", "binding-unknown", "auth-error", "server-error"} {
		t.Run(failure, func(t *testing.T) {
			var posts atomic.Int32
			var discovered atomic.Bool
			client := newControlsTestClient(t, config.SystemTypeFusion, func(w http.ResponseWriter, req *http.Request) {
				switch {
				case req.Method == http.MethodPost:
					posts.Add(1)
					if failure == "server-error" {
						http.Error(w, "unknown outcome", 503)
					} else {
						_, _ = io.WriteString(w, `{"errorCode":-44101}`)
					}
				case strings.HasSuffix(req.URL.Path, "/devices"):
					writePoEResult(t, w, []map[string]any{{"mac": poeTestMAC, "type": "switch", "status": 14}})
				case strings.HasSuffix(req.URL.Path, "/ports"):
					port := poeTestSwitchPort()
					if discovered.Load() && failure == "poe-disabled-after-discovery" {
						port["portStatus"].(map[string]any)["poe"] = false
					}
					writePoEResult(t, w, []map[string]any{port})
				default:
					writePoEResult(t, w, poeTestDetail("switch"))
				}
			})
			if targets, err := client.PoEControlTargets(t.Context()); err != nil || len(targets) != 1 {
				t.Fatalf("discovery: %+v %v", targets, err)
			}
			discovered.Store(true)
			binding := ""
			if failure == "binding-unknown" {
				binding = "00-11-22-33-44-55"
			}
			if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, 3, binding); err == nil {
				t.Fatal("failed/rebound PoE action reported success")
			}
			want := int32(1)
			if failure == "poe-disabled-after-discovery" || failure == "binding-unknown" {
				want = 0
			}
			if posts.Load() != want {
				t.Fatalf("POSTs=%d, want %d (no off/on or retry)", posts.Load(), want)
			}
		})
	}
}

func TestPoEUnsupportedOwnerDoesNotHideSupportedOwner(t *testing.T) {
	client := newControlsTestClient(t, config.SystemTypeStandard, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/devices"):
			_, _ = fmt.Fprintf(w, `{"errorCode":0,"result":[{"mac":"00-11-22-33-44-55","type":"gateway","status":14},{"mac":%q,"type":"switch","status":14}]}`, poeTestMAC)
		case strings.Contains(req.URL.Path, "/gateways/"):
			http.Error(w, "unsupported", 404)
		case strings.HasSuffix(req.URL.Path, "/ports"):
			writePoEResult(t, w, []map[string]any{poeTestSwitchPort()})
		default:
			writePoEResult(t, w, poeTestDetail("switch"))
		}
	})
	if targets, err := client.PoEControlTargets(t.Context()); err != nil || len(targets) != 1 || targets[0].MAC != poeTestMAC {
		t.Fatalf("supported owner lost: %+v %v", targets, err)
	}
}
