package omada

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

const syntheticAPMAC = "02-00-00-00-00-01"

func newAPUplinkTestClient(t *testing.T, ttl int, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/info":
			_, _ = fmt.Fprint(w, `{"errorCode":0,"result":{"omadacId":"cid"}}`)
		case "/cid/api/v2/loginStatus":
			_, _ = fmt.Fprint(w, `{"errorCode":0,"result":{"login":true}}`)
		case "/cid/api/v2/users/current":
			_, _ = fmt.Fprint(w, `{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`)
		default:
			handler(w, req)
		}
	}))
	t.Cleanup(server.Close)
	apiClient, err := session.Configure(&config.Config{
		Host: server.URL, Username: "user", Password: "pass", Site: "Default",
		SystemType: config.SystemTypeStandard, OpenAPIAuth: config.OpenAPIAuthWebSession,
		CacheTTL: ttl, Timeout: 5,
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	return NewClient(apiClient)
}

func TestGetAPUplinkStatsDecodesNestedCounters(t *testing.T) {
	client := newAPUplinkTestClient(t, 0, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/openapi/v1/cid/sites/site-id/aps/"+syntheticAPMAC+"/wired-uplink" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		if req.Header.Get("Omada-Request-Source") != "web-local" {
			t.Error("request did not use configured web-session OpenAPI authentication")
		}
		_, _ = fmt.Fprint(w, `{"errorCode":0,"result":{"wiredUplink":{"upPackets":1001,"downPackets":2002,"upDropPackets":0,"downDropPackets":4,"upErrorsPackets":5,"downErrorsPackets":6}}}`)
	})
	stats, err := client.GetAPUplinkStats(syntheticAPMAC)
	if err != nil || stats == nil {
		t.Fatalf("GetAPUplinkStats() = %v, %v", stats, err)
	}
	for name, counter := range map[string]struct {
		got  *uint64
		want uint64
	}{
		"upPackets": {stats.UpPackets, 1001}, "downPackets": {stats.DownPackets, 2002},
		"upDropPackets": {stats.UpDropPackets, 0}, "downDropPackets": {stats.DownDropPackets, 4},
		"upErrorsPackets": {stats.UpErrorPackets, 5}, "downErrorsPackets": {stats.DownErrorPackets, 6},
	} {
		if counter.got == nil || *counter.got != counter.want {
			t.Errorf("%s = %v, want %d", name, counter.got, counter.want)
		}
	}
}

func TestGetAPUplinkStatsUnavailable(t *testing.T) {
	for _, result := range []string{`null`, `{}`, `{"wiredUplink":null}`, `{"wiredUplink":{}}`, `{"wiredUplink":{"upPackets":null,"downPackets":null}}`, `{"wiredUplink":{"type":"switch","rate":"1000"}}`} {
		t.Run(result, func(t *testing.T) {
			client := newAPUplinkTestClient(t, 0, func(w http.ResponseWriter, req *http.Request) {
				_, _ = fmt.Fprintf(w, `{"errorCode":0,"result":%s}`, result)
			})
			stats, err := client.GetAPUplinkStats(syntheticAPMAC)
			if err != nil || stats != nil {
				t.Fatalf("GetAPUplinkStats() = %v, %v; want nil, nil", stats, err)
			}
		})
	}
}

func TestGetAPUplinkStatsKeepsErrorsDistinctFromUnavailable(t *testing.T) {
	for name, test := range map[string]struct {
		status int
		body   string
	}{
		"HTTP error":         {http.StatusNotFound, `{"errorCode":-1,"msg":"not supported"}`},
		"API error":          {http.StatusOK, `{"errorCode":-1,"msg":"not supported"}`},
		"invalid JSON":       {http.StatusOK, `{"result":`},
		"negative counter":   {http.StatusOK, `{"errorCode":0,"result":{"wiredUplink":{"upPackets":-1}}}`},
		"wrong result shape": {http.StatusOK, `{"errorCode":0,"result":[]}`},
	} {
		t.Run(name, func(t *testing.T) {
			client := newAPUplinkTestClient(t, 0, func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(test.status)
				_, _ = fmt.Fprint(w, test.body)
			})
			stats, err := client.GetAPUplinkStats(syntheticAPMAC)
			if err == nil || stats != nil {
				t.Fatalf("GetAPUplinkStats() = %v, %v; want nil and error", stats, err)
			}
		})
	}
}

func TestGetAPUplinkStatsCacheIsPerAP(t *testing.T) {
	var requests atomic.Int32
	client := newAPUplinkTestClient(t, 60, func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		if strings.Contains(req.URL.Path, syntheticAPMAC) {
			_, _ = fmt.Fprint(w, `{"errorCode":0,"result":{"wiredUplink":{"upPackets":0}}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"errorCode":0,"result":{}}`)
	})
	for range 2 {
		stats, err := client.GetAPUplinkStats(syntheticAPMAC)
		if err != nil || stats == nil || stats.UpPackets == nil || *stats.UpPackets != 0 {
			t.Fatalf("populated GetAPUplinkStats() = %v, %v", stats, err)
		}
		stats, err = client.GetAPUplinkStats("02-00-00-00-00-02")
		if err != nil || stats != nil {
			t.Fatalf("unavailable GetAPUplinkStats() = %v, %v", stats, err)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2 (one per AP, including unavailable data)", requests.Load())
	}
}

func TestGetAPUplinkStatsDoesNotCacheFailures(t *testing.T) {
	var requests atomic.Int32
	client := newAPUplinkTestClient(t, 60, func(w http.ResponseWriter, req *http.Request) {
		if requests.Add(1) == 1 {
			_, _ = fmt.Fprint(w, `{"errorCode":-1,"msg":"temporary failure"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"errorCode":0,"result":{"wiredUplink":{"downDropPackets":0}}}`)
	})
	if _, err := client.GetAPUplinkStats(syntheticAPMAC); err == nil {
		t.Fatal("first request must report the API error")
	}
	stats, err := client.GetAPUplinkStats(syntheticAPMAC)
	if err != nil || stats == nil || stats.DownDropPackets == nil || requests.Load() != 2 {
		t.Fatalf("retry = %v, %v; requests = %d", stats, err, requests.Load())
	}
}

func TestGetAPUplinkStatsRejectsEmptyMACAndDisabledAuth(t *testing.T) {
	client := newAPUplinkTestClient(t, 0, func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("unexpected request: %s", req.URL.Path)
	})
	if _, err := client.GetAPUplinkStats(" "); err == nil {
		t.Fatal("empty MAC address must be rejected")
	}
	client.session.OpenAPIAuthMode = config.OpenAPIAuthDisabled
	if _, err := client.GetAPUplinkStats(syntheticAPMAC); err == nil {
		t.Fatal("disabled OpenAPI authentication must be an error")
	}
}
