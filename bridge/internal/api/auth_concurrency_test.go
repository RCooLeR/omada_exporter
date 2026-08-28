package api

import (
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestWebAuthTransitionsAreSerialized(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		loginStatusStarted := make(chan struct{})
		releaseLoginStatus := make(chan struct{})
		infoStarted := make(chan struct{}, 1)
		client := &Client{
			Config: &config.Config{
				Host:     "https://omada.example",
				Site:     "Default",
				Username: "operator",
				Password: "secret",
			},
			requestCache: map[string]cacheEntry{},
			httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/old-cid/api/v2/loginStatus":
					close(loginStatusStarted)
					<-releaseLoginStatus
					return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
				case "/api/info":
					infoStarted <- struct{}{}
					return jsonResponse(`{"errorCode":0,"result":{"omadacId":"new-cid"}}`), nil
				case "/new-cid/api/v2/login":
					return jsonResponse(`{"errorCode":0,"result":{"token":"new-web-token"}}`), nil
				case "/new-cid/api/v2/users/current":
					return jsonResponse(`{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"new-site"}]}}}`), nil
				default:
					t.Fatalf("unexpected web auth request %s", req.URL.Path)
					return nil, nil
				}
			})},
		}
		client.SetContextIDs("old-cid", "old-site")
		client.setWebToken("old-web-token")

		ensureDone := make(chan error, 1)
		go func() { ensureDone <- client.ensureLoggedIn() }()
		<-loginStatusStarted

		reauthDone := make(chan error, 1)
		go func() { reauthDone <- client.reauthenticateWebSession() }()
		synctest.Wait()

		select {
		case <-infoStarted:
			t.Error("web reauthentication started while login-status transition was active")
		default:
		}
		close(releaseLoginStatus)
		if err := <-ensureDone; err != nil {
			t.Fatalf("ensureLoggedIn() error = %v", err)
		}
		if err := <-reauthDone; err != nil {
			t.Fatalf("reauthenticateWebSession() error = %v", err)
		}

		cid, siteID := client.ContextIDs()
		if cid != "new-cid" || siteID != "new-site" {
			t.Errorf("context IDs = (%q, %q), want new-cid/new-site", cid, siteID)
		}
		if token := client.currentWebToken(); token != "new-web-token" {
			t.Errorf("web token = %q, want new-web-token", token)
		}
	})
}

func TestOpenAPIAuthTransitionsAreSerialized(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		firstLoginStarted := make(chan struct{})
		releaseFirstLogin := make(chan struct{})
		infoStarted := make(chan struct{}, 1)
		var loginCalls atomic.Int32
		client := &Client{
			Config: &config.Config{
				Host:     "https://omada.example",
				ClientId: "client-id",
				SecretId: "client-secret",
			},
			OpenAPIAuthMode: config.OpenAPIAuthClientCredentials,
			requestCache:    map[string]cacheEntry{},
			httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/openapi/authorize/token":
					if loginCalls.Add(1) == 1 {
						close(firstLoginStarted)
						<-releaseFirstLogin
						return jsonResponse(`{"errorCode":0,"result":{"accessToken":"old-access","refreshToken":"old-refresh","expiresIn":3600}}`), nil
					}
					return jsonResponse(`{"errorCode":0,"result":{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":3600}}`), nil
				case "/api/info":
					infoStarted <- struct{}{}
					return jsonResponse(`{"errorCode":0,"result":{"omadacId":"new-cid"}}`), nil
				default:
					t.Fatalf("unexpected OpenAPI auth request %s", req.URL.Path)
					return nil, nil
				}
			})},
		}
		client.SetContextIDs("old-cid", "site-id")

		ensureDone := make(chan error, 1)
		go func() { ensureDone <- client.ensureOpenAPIAccessToken() }()
		<-firstLoginStarted

		reauthDone := make(chan error, 1)
		go func() { reauthDone <- client.reauthenticateOpenAPISession() }()
		synctest.Wait()

		select {
		case <-infoStarted:
			t.Error("OpenAPI reauthentication started while token login was active")
		default:
		}
		close(releaseFirstLogin)
		if err := <-ensureDone; err != nil {
			t.Fatalf("ensureOpenAPIAccessToken() error = %v", err)
		}
		if err := <-reauthDone; err != nil {
			t.Fatalf("reauthenticateOpenAPISession() error = %v", err)
		}

		cid, siteID := client.ContextIDs()
		if cid != "new-cid" || siteID != "site-id" {
			t.Errorf("context IDs = (%q, %q), want new-cid/site-id", cid, siteID)
		}
		accessToken, refreshToken, _ := client.currentOpenAPITokenState()
		if accessToken != "new-access" || refreshToken != "new-refresh" {
			t.Errorf("OpenAPI tokens = (%q, %q), want new-access/new-refresh", accessToken, refreshToken)
		}
		if calls := loginCalls.Load(); calls != 2 {
			t.Errorf("OpenAPI login calls = %d, want 2", calls)
		}
	})
}
