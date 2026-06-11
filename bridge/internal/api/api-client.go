package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

// Client coordinates authenticated access to the Omada APIs.
type Client struct {
	Config               *config.Config
	httpClient           *http.Client
	token                string
	OmadaCID             string
	SiteId               string
	authMu               sync.RWMutex
	accessToken          string
	refreshToken         string
	accessTokenExpiresAt time.Time
	cacheMu              sync.RWMutex
	requestCache         map[string]cacheEntry
	requestGroup         singleflight.Group
	webAuthGroup         singleflight.Group
	openAPIAuthGroup     singleflight.Group
}

// createHttpClient builds the shared HTTP client with TLS and timeout settings.
func createHttpClient(insecure bool, timeout int) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to init cookiejar")
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxConnsPerHost = 100
	t.MaxIdleConnsPerHost = 100

	client := &http.Client{Transport: t, Timeout: time.Duration(timeout) * time.Second, Jar: jar}

	if insecure {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return client, nil
}

// Configure creates an API client from the exporter configuration.
func Configure(c *config.Config) (*Client, error) {
	httpClient, err := createHttpClient(c.Insecure, c.Timeout)
	if err != nil {
		return nil, err
	}

	client := &Client{
		Config:       c,
		httpClient:   httpClient,
		requestCache: map[string]cacheEntry{},
	}
	cid, err := client.getCid()
	if err != nil {
		return nil, err
	}
	client.OmadaCID = cid

	sid, err := client.getSiteId(c.Site)
	if err != nil {
		return nil, err
	}
	client.SiteId = *sid
	if client.Config.ClientId != "" && client.Config.SecretId != "" {
		err := client.LoginOpenApi()
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

// makeRequest executes an HTTP request with the configured client.
func (c *Client) makeRequest(req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "omada_exporter")
	req.Header.Set("Connection", "keep-alive")

	if token := c.currentWebToken(); token != "" {
		req.Header.Set("Csrf-Token", token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, redactError(err)
	}
	return resp, nil
}

// cloneRequest copies an HTTP request so it can be retried safely.
func cloneRequest(req *http.Request) (*http.Request, error) {
	cloned := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return cloned, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("request body is not replayable")
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	cloned.Body = body
	return cloned, nil
}

// readAndRestoreBody reads a response body and restores it for later use.
func readAndRestoreBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return body, nil
}

// currentWebToken returns the CSRF token guarded by authMu.
//
// A single exporter scrape can touch several collectors at once. Those
// collectors share one API client, so credential fields must be read and
// written under a lock to keep retries and background MQTT publishing from
// racing each other.
func (c *Client) currentWebToken() string {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.token
}

func (c *Client) setWebToken(token string) {
	c.authMu.Lock()
	c.token = token
	c.authMu.Unlock()
}

func (c *Client) currentOpenAPITokenState() (string, string, time.Time) {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.accessToken, c.refreshToken, c.accessTokenExpiresAt
}

func (c *Client) setOpenAPITokens(accessToken, refreshToken string, expiresIn int32) {
	lifetime := time.Duration(expiresIn) * time.Second
	if lifetime > 5*time.Second {
		lifetime -= 5 * time.Second
	}

	c.authMu.Lock()
	c.accessToken = accessToken
	c.refreshToken = refreshToken
	c.accessTokenExpiresAt = time.Now().Add(lifetime)
	c.authMu.Unlock()
}

func (c *Client) clearOpenAPITokens() {
	c.authMu.Lock()
	c.accessToken = ""
	c.refreshToken = ""
	c.accessTokenExpiresAt = time.Time{}
	c.authMu.Unlock()
}

func (c *Client) currentOpenAPIAccessToken() string {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.accessToken
}

// redactURLString removes secrets from URLs before they are logged or returned.
func redactURLString(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsed.Query()
	for key := range query {
		switch strings.ToLower(key) {
		case "access_token", "client_secret", "password", "refresh_token", "token":
			query.Set(key, "<redacted>")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func redactError(err error) error {
	if err == nil {
		return nil
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		redacted := *urlErr
		redacted.URL = redactURLString(urlErr.URL)
		return &redacted
	}

	return err
}

// apiErrorResponse represents an error payload returned by Omada.
type apiErrorResponse struct {
	ErrorCode      *int   `json:"errorCode"`
	ErrorCodeSnake *int   `json:"error_code"`
	Msg            string `json:"msg"`
	ErrorMsg       string `json:"errorMsg"`
}

// isHTTPAuthStatus reports whether the status code indicates an authentication failure.
func isHTTPAuthStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

// isAuthRelatedMessage reports whether a message points to an authentication problem.
func isAuthRelatedMessage(message string) bool {
	message = strings.ToLower(message)
	if message == "" {
		return false
	}

	return strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "forbidden") ||
		(strings.Contains(message, "token") && (strings.Contains(message, "expired") || strings.Contains(message, "invalid") || strings.Contains(message, "missing"))) ||
		(strings.Contains(message, "auth") && (strings.Contains(message, "fail") || strings.Contains(message, "expired")))
}

// isWebAPIAuthFailure inspects a response for Web API authentication failures.
func isWebAPIAuthFailure(resp *http.Response) (bool, error) {
	if resp == nil {
		return false, nil
	}
	if isHTTPAuthStatus(resp.StatusCode) {
		return true, nil
	}

	body, err := readAndRestoreBody(resp)
	if err != nil {
		return false, err
	}

	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return false, nil
	}

	if apiErr.ErrorCode != nil && *apiErr.ErrorCode == -1200 {
		return true, nil
	}
	if apiErr.ErrorCodeSnake != nil && *apiErr.ErrorCodeSnake == -1200 {
		return true, nil
	}

	return isAuthRelatedMessage(apiErr.Msg) || isAuthRelatedMessage(apiErr.ErrorMsg), nil
}

// isOpenAPIAuthFailure inspects a response for Open API authentication failures.
func isOpenAPIAuthFailure(resp *http.Response) (bool, error) {
	if resp == nil {
		return false, nil
	}
	if isHTTPAuthStatus(resp.StatusCode) {
		return true, nil
	}

	body, err := readAndRestoreBody(resp)
	if err != nil {
		return false, err
	}

	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return false, nil
	}

	if apiErr.ErrorCode != nil && isHTTPAuthStatus(*apiErr.ErrorCode) {
		return true, nil
	}
	if apiErr.ErrorCodeSnake != nil && isHTTPAuthStatus(*apiErr.ErrorCodeSnake) {
		return true, nil
	}

	return isAuthRelatedMessage(apiErr.Msg) || isAuthRelatedMessage(apiErr.ErrorMsg), nil
}

// doLoggedInRequest performs a request using the current web session.
func (c *Client) doLoggedInRequest(req *http.Request) (*http.Response, error) {
	cloned, err := cloneRequest(req)
	if err != nil {
		return nil, err
	}
	return c.makeRequest(cloned)
}

// ensureLoggedIn makes sure the web session is authenticated.
func (c *Client) ensureLoggedIn() error {
	// Only one goroutine should check/login at a time. Without singleflight,
	// a Prometheus scrape that touches several web API collectors can trigger
	// several identical login attempts when the session expires.
	_, err, _ := c.webAuthGroup.Do("web-auth", func() (any, error) {
		loggedIn, err := c.IsLoggedIn()
		if err != nil {
			return nil, err
		}
		if !loggedIn {
			log.Info().Str("user", c.Config.Username).Msg("not logged in, logging in")
			if err := c.Login(); err != nil || c.currentWebToken() == "" {
				log.Error().Err(err).Msg("failed to login")
				return nil, err
			}
		}
		return nil, nil
	})
	return err
}

// reauthenticateWebSession refreshes the web session after authentication expires.
func (c *Client) reauthenticateWebSession() error {
	// Reauthentication also refreshes controller/site context because some
	// Omada controllers issue session-scoped IDs. The cache is cleared after a
	// successful refresh so callers do not keep serving data fetched with the
	// old session context.
	_, err, _ := c.webAuthGroup.Do("web-reauth", func() (any, error) {
		if err := c.RefreshOmadaContext(); err != nil {
			return nil, err
		}
		if err := c.Login(); err != nil {
			return nil, err
		}

		siteID, err := c.getSiteIdFromCurrentSession(c.Config.Site)
		if err != nil {
			return nil, err
		}
		c.SiteId = *siteID
		c.invalidateRequestCache()
		return nil, nil
	})
	return err
}

// MakeLoggedInRequest performs a web API request and retries after reauthentication when needed.
func (c *Client) MakeLoggedInRequest(req *http.Request) (*http.Response, error) {
	if err := c.ensureLoggedIn(); err != nil {
		return nil, err
	}
	log.Info().Str("url", redactURLString(req.URL.String())).Msg("MakeLoggedInRequest")

	resp, err := c.doLoggedInRequest(req)
	if err != nil {
		log.Warn().Err(err).Msg("request failed, re-authenticating web session")
		if reauthErr := c.reauthenticateWebSession(); reauthErr != nil {
			return nil, fmt.Errorf("request failed: %v; re-authentication failed: %w", err, reauthErr)
		}
		resp, err = c.doLoggedInRequest(req)
		if err != nil {
			return nil, err
		}
		authFailed, err := isWebAPIAuthFailure(resp)
		if err != nil {
			return nil, err
		}
		if authFailed {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("request remained unauthorized after re-authentication")
		}
		return resp, nil
	}

	authFailed, err := isWebAPIAuthFailure(resp)
	if err != nil {
		return nil, err
	}
	if !authFailed {
		return resp, nil
	}

	// Omada sometimes returns a JSON auth error with HTTP 200. We inspect the
	// body, close the failed response, refresh the session, and replay the
	// original request once.
	_ = resp.Body.Close()
	log.Warn().Msg("web session expired during request, re-authenticating")
	if err := c.reauthenticateWebSession(); err != nil {
		return nil, err
	}

	resp, err = c.doLoggedInRequest(req)
	if err != nil {
		return nil, err
	}
	authFailed, err = isWebAPIAuthFailure(resp)
	if err != nil {
		return nil, err
	}
	if authFailed {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request remained unauthorized after re-authentication")
	}

	return resp, nil
}

// doOpenAPIRequest performs a request using the current Open API token.
func (c *Client) doOpenAPIRequest(req *http.Request) (*http.Response, error) {
	cloned, err := cloneRequest(req)
	if err != nil {
		return nil, err
	}

	cloned.Header.Set("Accept", "application/json")
	cloned.Header.Set("X-Requested-With", "XMLHttpRequest")
	cloned.Header.Set("User-Agent", "omada_exporter")
	cloned.Header.Set("Connection", "keep-alive")
	cloned.Header.Set("Authorization", "AccessToken="+c.currentOpenAPIAccessToken())

	resp, err := c.httpClient.Do(cloned)
	if err != nil {
		return nil, redactError(err)
	}
	return resp, nil
}

// ensureOpenAPIAccessToken makes sure the Open API token is available.
func (c *Client) ensureOpenAPIAccessToken() error {
	// The Open API uses access/refresh tokens instead of the web CSRF token.
	// The refresh/login work is deduplicated for the same reason as web login:
	// a single scrape can need several Open API collectors at the same time.
	_, err, _ := c.openAPIAuthGroup.Do("openapi-auth", func() (any, error) {
		accessToken, refreshToken, expiresAt := c.currentOpenAPITokenState()
		now := time.Now()
		if now.After(expiresAt) && refreshToken != "" {
			if err := c.RefreshOpenApiToken(); err != nil {
				log.Warn().Err(err).Msg("failed to refresh OpenAPI token, requesting a new one")
				c.clearOpenAPITokens()
			}
		}

		accessToken, _, expiresAt = c.currentOpenAPITokenState()
		if expiresAt.IsZero() || time.Now().After(expiresAt) || accessToken == "" {
			return nil, c.LoginOpenApi()
		}

		return nil, nil
	})
	return err
}

// reauthenticateOpenAPISession refreshes the Open API session after authentication expires.
func (c *Client) reauthenticateOpenAPISession() error {
	_, err, _ := c.openAPIAuthGroup.Do("openapi-reauth", func() (any, error) {
		if err := c.RefreshOmadaContext(); err != nil {
			return nil, err
		}

		c.clearOpenAPITokens()
		if err := c.LoginOpenApi(); err != nil {
			return nil, err
		}
		c.invalidateRequestCache()
		return nil, nil
	})
	return err
}

// MakeOpenApiRequest performs an Open API request and retries after reauthentication when needed.
func (c *Client) MakeOpenApiRequest(req *http.Request) (*http.Response, error) {
	if err := c.ensureOpenAPIAccessToken(); err != nil {
		return nil, err
	}

	resp, err := c.doOpenAPIRequest(req)
	if err != nil {
		log.Warn().Err(err).Msg("OpenAPI request failed, re-authenticating")
		if reauthErr := c.reauthenticateOpenAPISession(); reauthErr != nil {
			return nil, fmt.Errorf("request failed: %v; re-authentication failed: %w", err, reauthErr)
		}
		resp, err = c.doOpenAPIRequest(req)
		if err != nil {
			return nil, err
		}
		authFailed, err := isOpenAPIAuthFailure(resp)
		if err != nil {
			return nil, err
		}
		if authFailed {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("request remained unauthorized after re-authentication")
		}
		return resp, nil
	}

	authFailed, err := isOpenAPIAuthFailure(resp)
	if err != nil {
		return nil, err
	}
	if !authFailed {
		return resp, nil
	}

	_ = resp.Body.Close()
	log.Warn().Msg("OpenAPI token expired during request, re-authenticating")
	if err := c.reauthenticateOpenAPISession(); err != nil {
		return nil, err
	}

	resp, err = c.doOpenAPIRequest(req)
	if err != nil {
		return nil, err
	}
	authFailed, err = isOpenAPIAuthFailure(resp)
	if err != nil {
		return nil, err
	}
	if authFailed {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request remained unauthorized after re-authentication")
	}

	return resp, nil
}
