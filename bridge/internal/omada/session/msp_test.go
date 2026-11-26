package session

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/testsupport/mspfake"
)

func mspTestConfig(fake *mspfake.Controller) *config.Config {
	return &config.Config{
		Host:       fake.URL(),
		Username:   fake.Username,
		Password:   fake.Password,
		ClientId:   fake.ClientID,
		SecretId:   fake.ClientSecret,
		SystemType: config.SystemTypeAuto,
		Site:       "Default",
		Timeout:    5,
	}
}

func withMSPReauthInterval(t *testing.T, interval time.Duration) {
	t.Helper()
	previous := mspReauthMinInterval
	mspReauthMinInterval = interval
	t.Cleanup(func() { mspReauthMinInterval = previous })
}

func isAuthRequest(r mspfake.Request) bool {
	return r.Path == "/openapi/authorize/token" || strings.HasSuffix(r.Path, "/api/v2/login") || strings.HasSuffix(r.Path, "/api/v2/loginStatus")
}

// assertAuthUsesMSPID checks the core MSP rule: authenticate with the MSP ID,
// and put the customer ID where the controller ID goes in data requests.
func assertAuthUsesMSPID(t *testing.T, requests []mspfake.Request, mspID string) {
	t.Helper()
	for _, r := range requests {
		switch {
		case r.Path == "/openapi/authorize/token":
			if strings.Contains(r.Query, "client_credentials") && !strings.Contains(r.Body, `"omadacId":"`+mspID+`"`) {
				t.Errorf("token request body %s does not use the MSP ID", r.Body)
			}
		case isAuthRequest(r):
			if !strings.HasPrefix(r.Path, "/"+mspID+"/") {
				t.Errorf("auth request %s %s does not use the MSP ID", r.Method, r.Path)
			}
		}
	}
}

func dataRequestsFor(requests []mspfake.Request, id string) int {
	count := 0
	for _, r := range requests {
		if isAuthRequest(r) || r.Path == "/api/info" {
			continue
		}
		if strings.HasPrefix(r.Path, "/"+id+"/") || strings.HasPrefix(r.Path, "/openapi/v1/"+id+"/") || strings.HasPrefix(r.Path, "/openapi/v2/"+id+"/") {
			count++
		}
	}
	return count
}

func TestConfigureDetectsMSPModeAndAuthenticatesWithMSPID(t *testing.T) {
	fake := mspfake.New(t)
	client, err := Configure(mspTestConfig(fake))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !client.MSPMode() || client.MSPID() != "msp-1" {
		t.Fatalf("MSPMode=%v MSPID=%q", client.MSPMode(), client.MSPID())
	}
	if client.ControllerKind != config.SystemTypeMSP || client.OpenAPIAuthMode != config.OpenAPIAuthClientCredentials {
		t.Fatalf("kind=%q openapi=%q", client.ControllerKind, client.OpenAPIAuthMode)
	}
	if cid, site := client.ContextIDs(); cid != "msp-1" || site != "" {
		t.Fatalf("root context = (%q, %q)", cid, site)
	}
	requests := fake.Requests()
	assertAuthUsesMSPID(t, requests, "msp-1")
	for _, r := range requests {
		if r.Path == "/msp-1/api/v2/users/current" {
			t.Fatal("MSP startup must not resolve a site through /{mspId}/api/v2/users/current")
		}
	}
	if web, open := fake.Logins(); web != 1 || open != 1 {
		t.Fatalf("logins web=%d openapi=%d, want 1/1", web, open)
	}
}

func TestConfigureForcedMSPWithoutControllerFlag(t *testing.T) {
	fake := mspfake.New(t)
	fake.SetReportMSPMode(false)
	conf := mspTestConfig(fake)
	conf.SystemType = config.SystemTypeMSP
	client, err := Configure(conf)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !client.MSPMode() {
		t.Fatal("OMADA_SYSTEM_TYPE=msp must force MSP mode")
	}
}

func TestConfigureMSPFailsClearlyWithoutOpenAPIClientCredentials(t *testing.T) {
	cases := map[string]func(*config.Config){
		"no client id":        func(c *config.Config) { c.ClientId = "" },
		"no secret":           func(c *config.Config) { c.SecretId = "" },
		"openapi disabled":    func(c *config.Config) { c.OpenAPIAuth = config.OpenAPIAuthDisabled },
		"openapi web session": func(c *config.Config) { c.OpenAPIAuth = config.OpenAPIAuthWebSession },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fake := mspfake.New(t)
			conf := mspTestConfig(fake)
			mutate(conf)
			_, err := Configure(conf)
			if err == nil || !strings.Contains(err.Error(), "MSP mode requires") {
				t.Fatalf("Configure error = %v, want an MSP requirement error", err)
			}
		})
	}
}

func TestConfigureMSPWithCustomerLevelAppIsRejected(t *testing.T) {
	fake := mspfake.New(t)
	conf := mspTestConfig(fake)
	conf.SecretId = "wrong"
	_, err := Configure(conf)
	if err == nil || !strings.Contains(err.Error(), "-44106") || !strings.Contains(err.Error(), "MSP level") {
		t.Fatalf("Configure error = %v, want -44106 with an MSP-level hint", err)
	}
}

func TestTargetViewUsesCustomerPathAndSharedMSPAuth(t *testing.T) {
	withMSPReauthInterval(t, 0)
	fake := mspfake.New(t)
	root, err := Configure(mspTestConfig(fake))
	if err != nil {
		t.Fatal(err)
	}
	view, err := root.ForTarget("cust-a", "site-a2", "Branch")
	if err != nil {
		t.Fatal(err)
	}
	if view.Settings().Site != "Branch" || root.Settings().Site != "Default" {
		t.Fatalf("view site %q root site %q", view.Settings().Site, root.Settings().Site)
	}

	webGet := func() {
		t.Helper()
		siteID, err := view.getSiteId("Branch")
		if err != nil {
			t.Fatalf("web request: %v", err)
		}
		if *siteID != "site-a2" {
			t.Fatalf("site = %q", *siteID)
		}
	}
	openGet := func() {
		t.Helper()
		cid, siteID := view.ContextIDs()
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/openapi/v1/%s/sites/%s/devices", fake.URL(), cid, siteID), nil)
		resp, err := view.MakeOpenApiRequest(req)
		if err != nil {
			t.Fatalf("openapi request: %v", err)
		}
		body, err := ReadResponseBody(resp, "devices")
		resp.Body.Close()
		if err != nil || ValidateAPIResponse(body, "devices") != nil {
			t.Fatalf("openapi response: %v %s", err, body)
		}
	}

	webGet()
	openGet()
	fake.ExpireWebSessions()
	fake.ExpireAccessTokens()
	webGet()
	openGet()

	requests := fake.Requests()
	assertAuthUsesMSPID(t, requests, "msp-1")
	if dataRequestsFor(requests, "msp-1") != 0 {
		t.Fatal("data requests must never use the MSP ID")
	}
	// 2 web + 2 OpenAPI calls; the OpenAPI call after token expiry is rejected
	// once (-44112) and replayed after the MSP re-login, again with cust-a.
	if dataRequestsFor(requests, "cust-a") != 5 {
		t.Fatalf("customer data requests = %d, want 5", dataRequestsFor(requests, "cust-a"))
	}
	if web, open := fake.Logins(); web != 2 || open != 2 {
		t.Fatalf("logins web=%d openapi=%d, want 2/2 (startup + one re-login each)", web, open)
	}
	if cid, site := view.ContextIDs(); cid != "cust-a" || site != "site-a2" {
		t.Fatalf("view context changed to (%q, %q)", cid, site)
	}
	if cid, site := root.ContextIDs(); cid != "msp-1" || site != "" {
		t.Fatalf("root context changed to (%q, %q)", cid, site)
	}
	if ok, failed := view.RequestStats(); ok != 4 || failed != 0 {
		t.Fatalf("request stats ok=%d failed=%d", ok, failed)
	}
}

func TestTargetViewAlertBodyUsesViewSite(t *testing.T) {
	fake := mspfake.New(t)
	root, err := Configure(mspTestConfig(fake))
	if err != nil {
		t.Fatal(err)
	}
	view, _ := root.ForTarget("cust-b", "site-b1", "Default")
	req, _ := http.NewRequest("POST", fake.URL()+"/cust-b/api/v2/sites/alert-count", strings.NewReader(`{"siteIds":["stale"]}`))
	resp, err := view.MakeLoggedInRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ReadResponseBody(resp, "alert")
	resp.Body.Close()
	if err := ValidateAPIResponse(body, "alert"); err != nil {
		t.Fatalf("alert-count: %v", err)
	}
	last := fake.Requests()[len(fake.Requests())-1]
	if last.Path != "/cust-b/api/v2/sites/alert-count" || !strings.Contains(last.Body, "site-b1") {
		t.Fatalf("alert request %s %s", last.Path, last.Body)
	}
}

func TestFailingCustomerCannotThrashSharedCredentials(t *testing.T) {
	fake := mspfake.New(t)
	root, err := Configure(mspTestConfig(fake))
	if err != nil {
		t.Fatal(err)
	}
	fake.SetForbidden("cust-b", true)
	bad, _ := root.ForTarget("cust-b", "site-b1", "Default")
	good, _ := root.ForTarget("cust-a", "site-a1", "Default")
	request := func(view *Client, cid, site string) error {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/openapi/v1/%s/sites/%s/clients", fake.URL(), cid, site), nil)
		resp, err := view.MakeOpenApiRequest(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := ReadResponseBody(resp, "clients")
		if err != nil {
			return err
		}
		return ValidateAPIResponse(body, "clients")
	}
	for range 5 {
		if err := request(bad, "cust-b", "site-b1"); err == nil {
			t.Fatal("forbidden customer request succeeded")
		}
		if err := request(good, "cust-a", "site-a1"); err != nil {
			t.Fatalf("healthy customer failed: %v", err)
		}
	}
	if _, open := fake.Logins(); open != 1 {
		t.Fatalf("OpenAPI logins = %d; a failing customer must not force repeated re-logins", open)
	}
	if ok, failed := bad.RequestStats(); ok != 0 || failed != 5 {
		t.Fatalf("bad stats ok=%d failed=%d", ok, failed)
	}
	if ok, failed := good.RequestStats(); ok != 5 || failed != 0 {
		t.Fatalf("good stats ok=%d failed=%d", ok, failed)
	}
}

func TestMSPRootOpenAPIPathKeepsMSPSegment(t *testing.T) {
	client := &Client{}
	client.SetContextIDs("msp-1", "")
	req, _ := http.NewRequest("GET", "https://controller/openapi/v1/msp/stale/customers?page=1", nil)
	got, err := client.requestWithCurrentContext(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/openapi/v1/msp/msp-1/customers" {
		t.Fatalf("path = %s", got.URL.Path)
	}
}

func TestForTargetRequiresMSPMode(t *testing.T) {
	client := &Client{Config: &config.Config{}}
	if _, err := client.ForTarget("cust-a", "site-a1", "Default"); err == nil {
		t.Fatal("ForTarget must fail outside MSP mode")
	}
}

func TestMSPRefusesDeviceControl(t *testing.T) {
	fake := mspfake.New(t)
	root, err := Configure(mspTestConfig(fake))
	if err != nil {
		t.Fatal(err)
	}
	view, _ := root.ForTarget("cust-a", "site-a1", "Default")
	err = view.WithWebControlSession(context.Background(), "site-a1", func(*WebControlSession) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "MSP") {
		t.Fatalf("control error = %v", err)
	}
}
