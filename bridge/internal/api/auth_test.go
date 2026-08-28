package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestLoginEncodesCredentialsAsJSON(t *testing.T) {
	username := "operator\"\\\nadmin"
	password := "pass\"\\word\nwith-newline"
	var payload map[string]string
	client := &Client{
		Config: &config.Config{
			Host:     "https://omada.example",
			Username: username,
			Password: password,
		},
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			decodeRequestJSON(t, req, &payload)
			return jsonResponse(`{"errorCode":0,"result":{"token":"web-token"}}`), nil
		})},
	}
	client.SetContextIDs("controller-id", "site-id")

	if err := client.Login(); err != nil {
		t.Fatalf("Login() returned error: %v", err)
	}
	if payload["username"] != username || payload["password"] != password {
		t.Fatalf("login payload = %#v, want original credentials", payload)
	}
	if len(payload) != 2 {
		t.Fatalf("login payload has %d fields, want 2: %#v", len(payload), payload)
	}
}

func TestLoginOpenAPIEncodesCredentialsAsJSON(t *testing.T) {
	omadaCID := "controller\"\\id"
	clientID := "client\"\\id"
	clientSecret := "secret\"\\value\nwith-newline"
	var payload map[string]string
	client := &Client{
		Config: &config.Config{
			Host:     "https://omada.example",
			ClientId: clientID,
			SecretId: clientSecret,
		},
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			decodeRequestJSON(t, req, &payload)
			return jsonResponse(`{"errorCode":0,"result":{"accessToken":"access-token","refreshToken":"refresh-token","expiresIn":3600}}`), nil
		})},
	}
	client.SetContextIDs(omadaCID, "site-id")

	if err := client.LoginOpenApi(); err != nil {
		t.Fatalf("LoginOpenApi() returned error: %v", err)
	}
	if payload["omadacId"] != omadaCID || payload["client_id"] != clientID || payload["client_secret"] != clientSecret {
		t.Fatalf("OpenAPI login payload = %#v, want original credentials", payload)
	}
	if len(payload) != 3 {
		t.Fatalf("OpenAPI login payload has %d fields, want 3: %#v", len(payload), payload)
	}
}

func TestIsLoggedInTreatsUnauthorizedStatusAsLoggedOut(t *testing.T) {
	client := &Client{
		Config: &config.Config{Host: "https://omada.example"},
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body:       http.NoBody,
			}, nil
		})},
	}
	client.SetContextIDs("controller-id", "site-id")

	loggedIn, err := client.IsLoggedIn()
	if err != nil {
		t.Fatalf("IsLoggedIn() error = %v", err)
	}
	if loggedIn {
		t.Fatal("IsLoggedIn() = true after HTTP 401")
	}
}

func decodeRequestJSON(t *testing.T, req *http.Request, target any) {
	t.Helper()

	if got := req.Header.Get("Content-Type"); got != "application/json; charset=UTF-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=UTF-8", got)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("request body is invalid JSON: %v; body: %q", err, body)
	}
}
