package openapi

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
)

func newWANQualityTestClient(t *testing.T, authMode string, handle http.HandlerFunc) *Client {
	t.Helper()
	var loggedIn atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/info":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"omadacId":"cid"}}`)
		case "/cid/api/v2/loginStatus":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"login":`+strconv.FormatBool(loggedIn.Load())+`}}`)
		case "/cid/api/v2/login":
			loggedIn.Store(true)
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"token":"test-web-token"}}`)
		case "/cid/api/v2/users/current":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`)
		case "/openapi/authorize/token":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"accessToken":"test-openapi-token","refreshToken":"test-refresh-token","expiresIn":3600}}`)
		default:
			if !strings.HasPrefix(req.URL.Path, "/openapi/v1/cid/sites/site-id/gateways/") {
				t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
				http.NotFound(w, req)
				return
			}
			if authMode == config.OpenAPIAuthWebSession {
				if req.Header.Get("Csrf-Token") != "test-web-token" || req.Header.Get("Omada-Request-Source") != "web-local" {
					t.Error("missing authenticated web-session headers")
				}
			} else if req.Header.Get("Authorization") != "AccessToken=test-openapi-token" {
				t.Error("missing OpenAPI access token header")
			}
			handle(w, req)
		}
	}))
	t.Cleanup(server.Close)
	client, err := api.Configure(&config.Config{
		Host: server.URL, Site: "Default", Username: "test-user", Password: "test-pass",
		ClientId: "test-client", SecretId: "test-secret", Timeout: 2, CacheTTL: 60,
		SystemType: config.SystemTypeStandard, OpenAPIAuth: authMode,
	})
	if err != nil {
		t.Fatalf("configure test client: %v", err)
	}
	return &Client{Client: client}
}

func TestGetWANQualityRequestsMillisecondsAndPreservesValues(t *testing.T) {
	for _, authMode := range []string{config.OpenAPIAuthClientCredentials, config.OpenAPIAuthWebSession} {
		t.Run(authMode, func(t *testing.T) {
			before := time.Now().UnixMilli()
			client := newWANQualityTestClient(t, authMode, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet || req.URL.Path != "/openapi/v1/cid/sites/site-id/gateways/AA-BB-CC-DD-EE-FF/health/detail" {
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
				}
				start, err := strconv.ParseInt(req.URL.Query().Get("start"), 10, 64)
				if err != nil {
					t.Errorf("invalid start timestamp: %v", err)
				}
				end, err := strconv.ParseInt(req.URL.Query().Get("end"), 10, 64)
				if err != nil || end < before || end > time.Now().UnixMilli() {
					t.Errorf("end is not current epoch milliseconds: %d, %v", end, err)
				}
				if end-start != 3600*1000 {
					t.Errorf("window = %d ms, want 3600000", end-start)
				}
				_, _ = io.WriteString(w, `{"errorCode":0,"result":{"wanStatus":{"support":true,"averageLatency":250.5,"averageJitter":0,"averagePktLoss":0.75,"averageMos":4.3}}}`)
			})
			got, err := client.GetWANQuality("AA-BB-CC-DD-EE-FF", 3600)
			if err != nil || got == nil {
				t.Fatalf("GetWANQuality() = %v, %v", got, err)
			}
			for name, value := range map[string]struct {
				got  *float64
				want float64
			}{
				"latency": {got.LatencyMilliseconds, 250.5}, "jitter": {got.JitterMilliseconds, 0},
				"packet loss": {got.PacketLossPercent, 0.75}, "MOS": {got.MOS, 4.3},
			} {
				if value.got == nil || *value.got != value.want {
					t.Errorf("%s = %v, want %v", name, value.got, value.want)
				}
			}
		})
	}
}

func TestGetWANQualityMissingAndUnsupportedValues(t *testing.T) {
	for name, body := range map[string]string{
		"missing result":     `{"errorCode":0}`,
		"null result":        `{"errorCode":0,"result":null}`,
		"missing WAN status": `{"errorCode":0,"result":{}}`,
		"null WAN status":    `{"errorCode":0,"result":{"wanStatus":null}}`,
		"unsupported":        `{"errorCode":0,"result":{"wanStatus":{"support":false,"averageLatency":25,"averagePktLoss":0}}}`,
		"missing support":    `{"errorCode":0,"result":{"wanStatus":{"averageLatency":25}}}`,
		"all absent":         `{"errorCode":0,"result":{"wanStatus":{"support":true}}}`,
		"all null":           `{"errorCode":0,"result":{"wanStatus":{"support":true,"averageLatency":null,"averageJitter":null,"averagePktLoss":null,"averageMos":null}}}`,
		"all negative":       `{"errorCode":0,"result":{"wanStatus":{"support":true,"averageLatency":-1,"averageJitter":-1,"averagePktLoss":-1,"averageMos":-1}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client := newWANQualityTestClient(t, config.OpenAPIAuthClientCredentials, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			})
			got, err := client.GetWANQuality("gateway", 3600)
			if err != nil || got != nil {
				t.Fatalf("GetWANQuality() = %v, %v; want nil, nil", got, err)
			}
		})
	}
}

func TestGetWANQualityPartialAndZeroValues(t *testing.T) {
	client := newWANQualityTestClient(t, config.OpenAPIAuthClientCredentials, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errorCode":0,"result":{"wanStatus":{"support":true,"averageLatency":null,"averageJitter":-1,"averagePktLoss":0}}}`)
	})
	got, err := client.GetWANQuality("gateway", 3600)
	if err != nil || got == nil {
		t.Fatalf("GetWANQuality() = %v, %v", got, err)
	}
	if got.LatencyMilliseconds != nil || got.JitterMilliseconds != nil || got.MOS != nil || got.PacketLossPercent == nil || *got.PacketLossPercent != 0 {
		t.Fatalf("missing values or legitimate zero incorrectly normalized: %#v", got)
	}
}

func TestGetWANQualityFailuresAreNotCached(t *testing.T) {
	for name, response := range map[string]struct {
		status int
		body   string
	}{
		"HTTP failure":      {http.StatusServiceUnavailable, `{"errorCode":0}`},
		"API failure":       {http.StatusOK, `{"errorCode":-1,"msg":"health unavailable"}`},
		"invalid JSON":      {http.StatusOK, `{"result":`},
		"wrong result type": {http.StatusOK, `{"errorCode":0,"result":[]}`},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			client := newWANQualityTestClient(t, config.OpenAPIAuthClientCredentials, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(response.status)
				_, _ = io.WriteString(w, response.body)
			})
			for range 2 {
				if got, err := client.GetWANQuality("gateway", 3600); err == nil || got != nil {
					t.Fatalf("GetWANQuality() = %v, %v; want an error", got, err)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("failed requests were cached: %d calls", calls.Load())
			}
		})
	}
}

func TestGetWANQualityCacheSeparatesGatewaysAndWindows(t *testing.T) {
	var calls atomic.Int32
	client := newWANQualityTestClient(t, config.OpenAPIAuthClientCredentials, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"errorCode":0,"result":{"wanStatus":{"support":true,"averageLatency":25}}}`)
	})
	for _, req := range []struct {
		mac    string
		window int
	}{{"gateway-a", 3600}, {"gateway-a", 3600}, {"gateway-b", 3600}, {"gateway-a", 7200}} {
		if _, err := client.GetWANQuality(req.mac, req.window); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("got %d fetches, want one per gateway/window", calls.Load())
	}
}

func TestGetWANQualityRejectsInvalidRequestsBeforeConnecting(t *testing.T) {
	client := &Client{Client: &api.Client{Config: &config.Config{}, OpenAPIAuthMode: config.OpenAPIAuthDisabled}}
	for _, window := range []int{0, -1, math.MaxInt} {
		if _, err := client.GetWANQuality("gateway", window); err == nil {
			t.Fatalf("window %d was accepted", window)
		}
	}
	if _, err := client.GetWANQuality("", 3600); err == nil {
		t.Fatal("missing gateway was accepted")
	}
	if _, err := client.GetWANQuality("gateway", 3600); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled OpenAPI must remain an error: %v", err)
	}
}

func TestNonNegativeWANMeasurementRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := nonNegativeWANMeasurement(&value); got != nil {
			t.Fatalf("accepted unavailable value %v", value)
		}
	}
}
