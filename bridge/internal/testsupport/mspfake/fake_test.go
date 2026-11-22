package mspfake

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

type envelope struct {
	ErrorCode int             `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// TestFakeReproducesProbeTable checks every row of the probe performed
// against a real MSP controller (v5.15.24.19, mspMode=true).
func TestFakeReproducesProbeTable(t *testing.T) {
	fake := New(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Jar: jar}
	var csrf, accessToken string

	call := func(t *testing.T, method, path string, body any, headers map[string]string) (int, envelope) {
		t.Helper()
		var reader *bytes.Reader
		if body != nil {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		} else {
			reader = bytes.NewReader(nil)
		}
		req, err := http.NewRequest(method, fake.URL()+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var decoded envelope
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, decoded
	}
	web := func() map[string]string { return map[string]string{"Csrf-Token": csrf} }
	open := func() map[string]string { return map[string]string{"Authorization": "AccessToken=" + accessToken} }
	wantCode := func(t *testing.T, got envelope, want int) {
		t.Helper()
		if got.ErrorCode != want {
			t.Fatalf("errorCode = %d (%s), want %d", got.ErrorCode, got.Msg, want)
		}
	}

	t.Run("GET /api/info", func(t *testing.T) {
		_, got := call(t, "GET", "/api/info", nil, nil)
		var info struct {
			OmadaCID string `json:"omadacId"`
			MSPMode  bool   `json:"mspMode"`
		}
		_ = json.Unmarshal(got.Result, &info)
		if info.OmadaCID != "msp-1" || !info.MSPMode {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("POST /{mspId}/api/v2/login", func(t *testing.T) {
		_, got := call(t, "POST", "/msp-1/api/v2/login", map[string]string{"username": fake.Username, "password": fake.Password}, nil)
		wantCode(t, got, 0)
		var result struct{ Token string }
		_ = json.Unmarshal(got.Result, &result)
		if result.Token == "" {
			t.Fatal("missing token")
		}
		csrf = result.Token
	})
	t.Run("GET /{mspId}/api/v2/loginStatus", func(t *testing.T) {
		_, got := call(t, "GET", "/msp-1/api/v2/loginStatus", nil, web())
		wantCode(t, got, 0)
		if string(got.Result) != `{"login":true}` {
			t.Fatalf("result = %s", got.Result)
		}
	})
	t.Run("GET /{mspId}/api/v2/users/current fails with -1", func(t *testing.T) {
		_, got := call(t, "GET", "/msp-1/api/v2/users/current", nil, web())
		wantCode(t, got, CodeGeneralError)
	})
	t.Run("GET /{mspId}/api/v2/settings/system/status", func(t *testing.T) {
		_, got := call(t, "GET", "/msp-1/api/v2/settings/system/status", nil, web())
		wantCode(t, got, 0)
	})
	t.Run("GET /{customerId}/api/v2/users/current with the MSP session", func(t *testing.T) {
		_, got := call(t, "GET", "/cust-a/api/v2/users/current", nil, web())
		wantCode(t, got, 0)
		if !bytes.Contains(got.Result, []byte(`"key":"site-a1"`)) {
			t.Fatalf("result = %s", got.Result)
		}
	})
	t.Run("GET /{customerId}/api/v2/sites/{siteId}/devices", func(t *testing.T) {
		_, got := call(t, "GET", "/cust-a/api/v2/sites/site-a1/devices", nil, web())
		wantCode(t, got, 0)
	})
	t.Run("GET /{customerId}/api/v2/settings/system/status", func(t *testing.T) {
		_, got := call(t, "GET", "/cust-a/api/v2/settings/system/status", nil, web())
		wantCode(t, got, 0)
		if !bytes.Contains(got.Result, []byte(`"name":"Alpha Corp"`)) {
			t.Fatalf("result = %s", got.Result)
		}
	})
	t.Run("token with omadacId=customerId fails with -44106", func(t *testing.T) {
		_, got := call(t, "POST", "/openapi/authorize/token?grant_type=client_credentials", map[string]string{"omadacId": "cust-a", "client_id": fake.ClientID, "client_secret": fake.ClientSecret}, nil)
		wantCode(t, got, CodeInvalidClient)
	})
	t.Run("token with omadacId=mspId", func(t *testing.T) {
		_, got := call(t, "POST", "/openapi/authorize/token?grant_type=client_credentials", map[string]string{"omadacId": "msp-1", "client_id": fake.ClientID, "client_secret": fake.ClientSecret}, nil)
		wantCode(t, got, 0)
		var result struct{ AccessToken string }
		_ = json.Unmarshal(got.Result, &result)
		accessToken = result.AccessToken
	})
	t.Run("GET /openapi/v1/msp/{mspId}/customers", func(t *testing.T) {
		_, got := call(t, "GET", "/openapi/v1/msp/msp-1/customers?page=1&pageSize=100", nil, open())
		wantCode(t, got, 0)
		if !bytes.Contains(got.Result, []byte(`"customerId":"cust-a"`)) {
			t.Fatalf("result = %s", got.Result)
		}
	})
	t.Run("GET /openapi/v1/{mspId}/sites fails with -1005", func(t *testing.T) {
		_, got := call(t, "GET", "/openapi/v1/msp-1/sites?page=1&pageSize=100", nil, open())
		wantCode(t, got, CodeOperationForbidden)
	})
	t.Run("GET /openapi/v1/{customerId}/sites with the MSP token", func(t *testing.T) {
		_, got := call(t, "GET", "/openapi/v1/cust-a/sites?page=1&pageSize=100", nil, open())
		wantCode(t, got, 0)
		if !bytes.Contains(got.Result, []byte(`"siteId":"site-a1"`)) {
			t.Fatalf("result = %s", got.Result)
		}
	})
	for _, endpoint := range []string{"devices", "clients", "vpn"} {
		t.Run("GET /openapi/v1/{customerId}/sites/{siteId}/"+endpoint, func(t *testing.T) {
			_, got := call(t, "GET", "/openapi/v1/cust-a/sites/site-a1/"+endpoint, nil, open())
			wantCode(t, got, 0)
		})
	}
	t.Run("OpenAPI with web session only fails with HTTP 401 -44116", func(t *testing.T) {
		status, got := call(t, "GET", "/openapi/v1/cust-a/sites?page=1&pageSize=100", nil, web())
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d", status)
		}
		wantCode(t, got, CodeOpenAPIUnauthorized)
	})
}
