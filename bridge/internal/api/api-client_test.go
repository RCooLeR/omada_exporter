package api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

func TestReadResponseBodyEnforcesLimit(t *testing.T) {
	response := jsonResponse("12345678")
	body, err := readResponseBody(response, "test", 8)
	if err != nil {
		t.Fatalf("readResponseBody(exact limit) error = %v", err)
	}
	if string(body) != "12345678" {
		t.Fatalf("readResponseBody(exact limit) = %q", body)
	}

	response = jsonResponse("123456789")
	_, err = readResponseBody(response, "test", 8)
	if err == nil || !strings.Contains(err.Error(), "exceeds 8 bytes") {
		t.Fatalf("readResponseBody(over limit) error = %v, want size error", err)
	}
}

func TestReadResponseBodyReportsStructuredHTTPError(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusBadGateway,
		Status:     "502 Bad Gateway",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"errorCode":-1600,"msg":"unsupported request path"}`)),
	}

	_, err := ReadResponseBody(response, "clients")
	if err == nil {
		t.Fatal("ReadResponseBody() returned nil error for HTTP 502")
	}
	for _, want := range []string{"clients", "502 Bad Gateway", "errorCode -1600", "unsupported request path"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ReadResponseBody() error = %q, want %q", err, want)
		}
	}
}

func TestRequestWithCurrentContextRefreshesSessionScopedIDs(t *testing.T) {
	client := &Client{}
	client.SetContextIDs("new-cid", "new-site")

	tests := []struct {
		name     string
		method   string
		url      string
		body     string
		wantPath string
		wantBody string
	}{
		{
			name:     "Web API path",
			method:   http.MethodGet,
			url:      "https://omada.example/controller/old-cid/api/v2/sites/old-site/devices",
			wantPath: "/controller/new-cid/api/v2/sites/new-site/devices",
		},
		{
			name:     "OpenAPI path",
			method:   http.MethodGet,
			url:      "https://omada.example/controller/openapi/v1/old-cid/sites/old-site/clients",
			wantPath: "/controller/openapi/v1/new-cid/sites/new-site/clients",
		},
		{
			name:     "alert request body",
			method:   http.MethodPost,
			url:      "https://omada.example/controller/old-cid/api/v2/sites/alert-count",
			body:     `{"siteIds":["old-site"]}`,
			wantPath: "/controller/new-cid/api/v2/sites/alert-count",
			wantBody: `{"siteIds":["new-site"]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(test.method, test.url, strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}
			contextual, err := client.requestWithCurrentContext(request)
			if err != nil {
				t.Fatalf("requestWithCurrentContext() error = %v", err)
			}
			if contextual.URL.Path != test.wantPath {
				t.Errorf("request path = %q, want %q", contextual.URL.Path, test.wantPath)
			}
			if test.wantBody != "" {
				body, err := io.ReadAll(contextual.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				if string(body) != test.wantBody {
					t.Errorf("request body = %q, want %q", body, test.wantBody)
				}
			}
		})
	}
}

func TestReadAndRestoreBodyPreservesSuccessfulBody(t *testing.T) {
	response := jsonResponse(`{"result":true}`)
	first, err := readAndRestoreBody(response)
	if err != nil {
		t.Fatalf("readAndRestoreBody() error = %v", err)
	}
	second, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading restored body error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("restored body = %q, want %q", second, first)
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
		requestCache: map[string]cacheEntry{},
	}
	client.SetContextIDs("cid", "site-id")
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

func TestContextIDsRemainAnAtomicPairDuringConcurrentRefresh(t *testing.T) {
	client := &Client{}
	client.SetContextIDs("cid-a", "site-a")

	const iterations = 100_000
	start := make(chan struct{})
	mismatches := make(chan [2]string, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := range iterations {
			if i%2 == 0 {
				client.SetContextIDs("cid-a", "site-a")
			} else {
				client.SetContextIDs("cid-b", "site-b")
			}
		}
	})
	for range 8 {
		wg.Go(func() {
			<-start
			for range iterations {
				cid, siteID := client.ContextIDs()
				if (cid == "cid-a" && siteID == "site-a") || (cid == "cid-b" && siteID == "site-b") {
					continue
				}
				select {
				case mismatches <- [2]string{cid, siteID}:
				default:
				}
				return
			}
		})
	}
	close(start)
	wg.Wait()

	select {
	case mismatch := <-mismatches:
		t.Fatalf("ContextIDs() returned mixed snapshot (%q, %q)", mismatch[0], mismatch[1])
	default:
	}
}

func TestMakeRequestUsesHTTPCompatibleHeaders(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Connection"); got != "" {
			t.Errorf("Connection header = %q, want empty for HTTP/2 compatibility", got)
		}
		if got := req.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept header = %q, want application/json", got)
		}
		return jsonResponse(`{"ok":true}`), nil
	})}}
	req, err := http.NewRequest(http.MethodGet, "https://omada.example/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	response, err := client.makeRequest(req)
	if err != nil {
		t.Fatalf("makeRequest() error = %v", err)
	}
	_ = response.Body.Close()
}

func contains(value, part string) bool {
	return strings.Contains(value, part)
}
