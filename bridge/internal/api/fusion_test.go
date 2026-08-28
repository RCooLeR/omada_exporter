package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestGetSiteIdUsesSingleSiteFallbackForAutoDefault(t *testing.T) {
	client := &Client{Config: &config.Config{SystemType: config.SystemTypeAuto}}

	siteID, err := client.getSiteIdWithRequest("Default", "controller-id", staticJSONResponse(`{
		"errorCode": 0,
		"result": {
			"privilege": {
				"sites": [{"name": "FUSION 2.5G_E9F148", "key": "fusion-site-id"}]
			}
		}
	}`))
	if err != nil {
		t.Fatalf("getSiteIdWithRequest() returned error: %v", err)
	}
	if siteID == nil || *siteID != "fusion-site-id" {
		t.Fatalf("site id = %v, want fusion-site-id", siteID)
	}
}

func TestGetSiteIdDoesNotFallbackForStandardController(t *testing.T) {
	client := &Client{Config: &config.Config{SystemType: config.SystemTypeStandard}}

	_, err := client.getSiteIdWithRequest("Default", "controller-id", staticJSONResponse(`{
		"errorCode": 0,
		"result": {
			"privilege": {
				"sites": [{"name": "FUSION 2.5G_E9F148", "key": "fusion-site-id"}]
			}
		}
	}`))
	if err == nil {
		t.Fatal("getSiteIdWithRequest() returned nil error, want missing site error")
	}
	if !strings.Contains(err.Error(), "available sites") {
		t.Fatalf("error = %q, want available site names", err)
	}
}

func TestDoOpenAPIRequestUsesWebSessionHeaders(t *testing.T) {
	var gotSource, gotCSRF, gotAuthorization string
	client := &Client{
		Config: &config.Config{},
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotSource = req.Header.Get("Omada-Request-Source")
			gotCSRF = req.Header.Get("Csrf-Token")
			gotAuthorization = req.Header.Get("Authorization")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"errorCode":0}`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})},
		OpenAPIAuthMode: config.OpenAPIAuthWebSession,
	}
	client.setWebToken("web-token")

	req, err := http.NewRequest("GET", "https://omada.example/openapi/v1/cid/aio/basic_info", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.doOpenAPIRequest(req)
	if err != nil {
		t.Fatalf("doOpenAPIRequest() returned error: %v", err)
	}
	_ = resp.Body.Close()

	if gotSource != "web-local" {
		t.Fatalf("Omada-Request-Source = %q, want web-local", gotSource)
	}
	if gotCSRF != "web-token" {
		t.Fatalf("Csrf-Token = %q, want web-token", gotCSRF)
	}
	if gotAuthorization != "" {
		t.Fatalf("Authorization = %q, want empty for web-session OpenAPI", gotAuthorization)
	}
}

func staticJSONResponse(body string) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
