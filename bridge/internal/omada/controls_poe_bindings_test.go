package omada

import (
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

const poeEndpointMAC = "00-11-22-33-44-55"

func poeEndpointRow(client bool) map[string]any {
	deviceType := 3
	if client {
		deviceType = 107
	}
	return map[string]any{
		"mac": poeEndpointMAC, "name": "Powered endpoint", "siteId": "site-id",
		"eligibleFlag": true, "clientFlag": client, "deviceType": deviceType,
		"uplinkDevice": map[string]any{"mac": poeTestMAC, "type": "switch", "uplinkType": 0, "uplinkPort": 3, "isStack": false},
	}
}

func newPoEBindingTestClient(t *testing.T, kind string, posts *atomic.Int32, candidates http.HandlerFunc) *Client {
	t.Helper()
	return newControlsTestClient(t, kind, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method != http.MethodGet:
			posts.Add(1)
			if req.Method != http.MethodPost || req.URL.Path != "/cid/api/v2/sites/site-id/cmd/switches/"+poeTestMAC+"/ports/3/restart" {
				t.Errorf("unexpected mutation: %s %s", req.Method, req.URL.Path)
			}
			writePoEResult(t, w, map[string]any{})
		case strings.HasSuffix(req.URL.Path, "/devices"):
			writePoEResult(t, w, []map[string]any{{"mac": poeTestMAC, "type": "switch", "status": 14}})
		case strings.Contains(req.URL.Path, "/eligible-"):
			query := req.URL.Query()
			if query.Get("currentPageSize") != "100" {
				t.Error("eligibility must use Web API currentPageSize")
			}
			if strings.Contains(req.URL.Path, "/monitor-client/") {
				if query.Get("filters.active") != "true" || query.Get("filters.wireless") != "false" || len(query) != 4 {
					t.Errorf("incorrect client filters: %s", req.URL.RawQuery)
				}
			} else if query.Get("filters.status") != "14,15,16,17" || query.Get("filters.deviceType") != "3" || len(query) != 4 {
				t.Errorf("incorrect device filters: %s", req.URL.RawQuery)
			}
			candidates(w, req)
		case strings.HasSuffix(req.URL.Path, "/ports"):
			writePoEResult(t, w, []map[string]any{poeTestSwitchPort()})
		case strings.HasSuffix(req.URL.Path, "/switches/"+poeTestMAC):
			writePoEResult(t, w, poeTestDetail("switch"))
		default:
			t.Errorf("unexpected read: %s", req.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	})
}

func writePoEEndpointPage(t *testing.T, w http.ResponseWriter, req *http.Request, total int, rows []map[string]any) {
	t.Helper()
	page, err := strconv.Atoi(req.URL.Query().Get("currentPage"))
	if err != nil || page < 1 {
		t.Fatal("eligibility must use currentPage pagination")
	}
	writePoEResult(t, w, map[string]any{"currentPage": page, "totalRows": total, "data": rows})
}

func TestPoEEndpointShortcutsUseControllerEligibilityForBothKinds(t *testing.T) {
	for _, kind := range []string{config.SystemTypeStandard, config.SystemTypeFusion} {
		for _, isClient := range []bool{false, true} {
			t.Run(kind+"/client="+strconv.FormatBool(isClient), func(t *testing.T) {
				var posts, pages atomic.Int32
				client := newPoEBindingTestClient(t, kind, &posts, func(w http.ResponseWriter, req *http.Request) {
					pages.Add(1)
					var rows []map[string]any
					if strings.Contains(req.URL.Path, "/monitor-client/") == isClient {
						rows = []map[string]any{poeEndpointRow(isClient)}
					}
					writePoEEndpointPage(t, w, req, len(rows), rows)
				})
				targets, err := client.PoEControlTargets(t.Context())
				if err != nil || len(targets) != 1 || targets[0].PoweredDeviceMAC != poeEndpointMAC || targets[0].PoweredDeviceName != "Powered endpoint" {
					t.Fatalf("binding: %+v, %v", targets, err)
				}
				if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, 3, "00:11:22:33:44:55"); err != nil {
					t.Fatal(err)
				}
				if posts.Load() != 1 || pages.Load() != 4 {
					t.Fatalf("posts=%d eligibility reads=%d; each command requires both fresh lists", posts.Load(), pages.Load())
				}
				if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, 3, ""); err == nil || posts.Load() != 1 {
					t.Fatal("stale unknown-endpoint command must not acquire a new endpoint binding")
				}
			})
		}
	}
}

func TestPoEEndpointShortcutsRejectMissingOrUnsafeProof(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"not-eligible", func(r map[string]any) { r["eligibleFlag"] = false }},
		{"missing-eligibility", func(r map[string]any) { delete(r, "eligibleFlag") }},
		{"missing-client-kind", func(r map[string]any) { delete(r, "clientFlag") }},
		{"wrong-client-kind", func(r map[string]any) { r["clientFlag"] = true }},
		{"wrong-device-kind", func(r map[string]any) { r["deviceType"] = 2 }},
		{"missing-device-kind", func(r map[string]any) { delete(r, "deviceType") }},
		{"foreign-site", func(r map[string]any) { r["siteId"] = "other" }},
		{"invalid-mac", func(r map[string]any) { r["mac"] = "invalid" }},
		{"owner-is-endpoint", func(r map[string]any) { r["mac"] = poeTestMAC }},
		{"wireless-backhaul", func(r map[string]any) { r["uplinkDevice"].(map[string]any)["uplinkType"] = 1 }},
		{"missing-backhaul", func(r map[string]any) { delete(r["uplinkDevice"].(map[string]any), "uplinkType") }},
		{"wrong-owner", func(r map[string]any) { r["uplinkDevice"].(map[string]any)["mac"] = "00-00-00-00-00-01" }},
		{"wrong-port", func(r map[string]any) { r["uplinkDevice"].(map[string]any)["uplinkPort"] = 4 }},
		{"wrong-owner-type", func(r map[string]any) { r["uplinkDevice"].(map[string]any)["type"] = "gateway" }},
		{"stack", func(r map[string]any) { r["uplinkDevice"].(map[string]any)["isStack"] = true }},
		{"stack-id", func(r map[string]any) { r["uplinkDevice"].(map[string]any)["stackId"] = "stack" }},
		{"stack-unit", func(r map[string]any) { r["uplinkDevice"].(map[string]any)["unit"] = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var posts atomic.Int32
			client := newPoEBindingTestClient(t, config.SystemTypeFusion, &posts, func(w http.ResponseWriter, req *http.Request) {
				var rows []map[string]any
				if strings.Contains(req.URL.Path, "/monitor-device/") {
					row := poeEndpointRow(false)
					test.edit(row)
					rows = append(rows, row)
				}
				writePoEEndpointPage(t, w, req, len(rows), rows)
			})
			targets, err := client.PoEControlTargets(t.Context())
			if err != nil || len(targets) != 1 || targets[0].PoweredDeviceMAC != "" {
				t.Fatalf("unsafe shortcut or lost explicit port: %+v %v", targets, err)
			}
			if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, 3, poeEndpointMAC); err == nil || posts.Load() != 0 {
				t.Fatal("unproven binding executed")
			}
		})
	}
}

func TestPoEEndpointPaginationAndAmbiguity(t *testing.T) {
	for _, mode := range []string{"clamped-pages", "duplicate-port", "duplicate-endpoint", "changed-count", "truncated-list", "wrong-page", "missing-metadata", "unsupported-clients"} {
		t.Run(mode, func(t *testing.T) {
			var posts atomic.Int32
			client := newPoEBindingTestClient(t, config.SystemTypeStandard, &posts, func(w http.ResponseWriter, req *http.Request) {
				if strings.Contains(req.URL.Path, "/monitor-client/") {
					if mode == "unsupported-clients" {
						http.Error(w, "unsupported", 404)
					} else {
						writePoEEndpointPage(t, w, req, 0, nil)
					}
					return
				}
				row := poeEndpointRow(false)
				page, _ := strconv.Atoi(req.URL.Query().Get("currentPage"))
				if page == 1 {
					switch mode {
					case "missing-metadata":
						writePoEResult(t, w, map[string]any{"data": []any{row}})
						return
					case "wrong-page":
						writePoEResult(t, w, map[string]any{"currentPage": 2, "totalRows": 1, "data": []any{row}})
						return
					case "unsupported-clients":
						writePoEEndpointPage(t, w, req, 1, []map[string]any{row})
						return
					}
					writePoEEndpointPage(t, w, req, 2, []map[string]any{row})
					return
				}
				if page != 2 {
					t.Fatal("unbounded eligibility pagination")
				}
				if mode == "truncated-list" {
					writePoEEndpointPage(t, w, req, 2, nil)
					return
				}
				if mode != "duplicate-endpoint" {
					row["mac"] = "00-11-22-33-44-66"
				}
				if mode != "duplicate-port" {
					row["uplinkDevice"].(map[string]any)["uplinkPort"] = 4
				}
				total := 2
				if mode == "changed-count" {
					total = 3
				}
				writePoEEndpointPage(t, w, req, total, []map[string]any{row})
			})
			targets, err := client.PoEControlTargets(t.Context())
			if err != nil || len(targets) != 1 {
				t.Fatalf("explicit port lost: %+v %v", targets, err)
			}
			want := ""
			if mode == "clamped-pages" {
				want = poeEndpointMAC
			}
			if targets[0].PoweredDeviceMAC != want || posts.Load() != 0 {
				t.Fatalf("binding=%q want=%q posts=%d", targets[0].PoweredDeviceMAC, want, posts.Load())
			}
		})
	}
}

func TestPoEEndpointBindingChangesRejectBeforeMutation(t *testing.T) {
	var posts atomic.Int32
	var changed atomic.Bool
	client := newPoEBindingTestClient(t, config.SystemTypeFusion, &posts, func(w http.ResponseWriter, req *http.Request) {
		var rows []map[string]any
		if strings.Contains(req.URL.Path, "/monitor-client/") {
			row := poeEndpointRow(true)
			if changed.Load() {
				row["mac"] = "00-11-22-33-44-66"
			}
			rows = append(rows, row)
		}
		writePoEEndpointPage(t, w, req, len(rows), rows)
	})
	targets, err := client.PoEControlTargets(t.Context())
	if err != nil || len(targets) != 1 || targets[0].PoweredDeviceMAC != poeEndpointMAC {
		t.Fatalf("discovery: %+v %v", targets, err)
	}
	changed.Store(true)
	if err := client.PowerCyclePoEPort(t.Context(), "site-id", poeTestMAC, 3, poeEndpointMAC); err == nil || posts.Load() != 0 {
		t.Fatal("stale endpoint binding executed")
	}
}
