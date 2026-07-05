package api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestRedactURLString(t *testing.T) {
	got := redactURLString("https://omada.example/openapi/authorize/token?client_id=id&client_secret=secret&refresh_token=refresh&grant_type=refresh_token")

	if got == "" {
		t.Fatal("redactURLString() returned empty URL")
	}
	for _, leaked := range []string{"client_secret=secret", "refresh_token=refresh"} {
		if contains(got, leaked) {
			t.Fatalf("redactURLString() = %q still contains %q", got, leaked)
		}
	}
	for _, expected := range []string{"client_id=id", "client_secret=%3Credacted%3E", "refresh_token=%3Credacted%3E"} {
		if !contains(got, expected) {
			t.Fatalf("redactURLString() = %q missing %q", got, expected)
		}
	}
}

func TestRedactErrorRedactsURLError(t *testing.T) {
	err := &url.Error{
		Op:  "Post",
		URL: "https://omada.example/openapi?token=abc123",
		Err: errors.New("connection refused"),
	}

	got := redactError(err).Error()
	if contains(got, "abc123") {
		t.Fatalf("redactError() = %q still contains token", got)
	}
	if !contains(got, "token=%3Credacted%3E") {
		t.Fatalf("redactError() = %q missing redacted token", got)
	}
}

func TestMakeLoggedInRequestDoesNotReauthenticateOnTransportError(t *testing.T) {
	var requestedPaths []string
	requestErr := &url.Error{Op: "Get", URL: "https://omada.example/resource", Err: errors.New("timeout")}
	client := &Client{
		Config: &config.Config{
			Host:     "https://omada.example",
			Username: "user",
			Password: "pass",
		},
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requestedPaths = append(requestedPaths, req.URL.Path)
				switch req.URL.Path {
				case "/cid/api/v2/loginStatus":
					return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
				case "/resource":
					return nil, requestErr
				default:
					t.Fatalf("unexpected reauthentication request to %s", req.URL.Path)
					return nil, nil
				}
			}),
		},
		OmadaCID:     "cid",
		requestCache: map[string]cacheEntry{},
	}
	req, err := http.NewRequest("GET", "https://omada.example/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest() returned error: %v", err)
	}

	_, err = client.MakeLoggedInRequest(req)
	if err == nil {
		t.Fatal("MakeLoggedInRequest() returned nil error, want transport error")
	}
	if strings.Contains(strings.Join(requestedPaths, ","), "/api/info") {
		t.Fatalf("MakeLoggedInRequest() reauthenticated after transport error; paths: %v", requestedPaths)
	}
}

func contains(value, part string) bool {
	return strings.Contains(value, part)
}
