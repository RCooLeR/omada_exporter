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

const maxAPIResponseBodyBytes int64 = 32 << 20

type authTransitionGate struct {
	once   sync.Once
	permit chan struct{}
}

func (g *authTransitionGate) lock() func() {
	g.once.Do(func() {
		g.permit = make(chan struct{}, 1)
		g.permit <- struct{}{}
	})
	<-g.permit
	return func() { g.permit <- struct{}{} }
}

// Client coordinates authenticated access to the Omada APIs.
type Client struct {
	Config               *config.Config
	httpClient           *http.Client
	token                string
	contextMu            sync.RWMutex
	omadaCID             string
	siteID               string
	ControllerKind       string
	OpenAPIAuthMode      string
	authMu               sync.RWMutex
	accessToken          string
	refreshToken         string
	accessTokenExpiresAt time.Time
	cacheMu              sync.RWMutex
	requestCache         map[string]cacheEntry
	cacheGeneration      uint64
	requestGroup         singleflight.Group
	webAuthTransition    authTransitionGate
	openAuthTransition   authTransitionGate
	webAuthGroup         singleflight.Group
	openAPIAuthGroup     singleflight.Group
}

// ContextIDs returns an atomic snapshot of the controller and site identifiers.
func (c *Client) ContextIDs() (omadaCID, siteID string) {
	c.contextMu.RLock()
	defer c.contextMu.RUnlock()
	return c.omadaCID, c.siteID
}

// SetContextIDs atomically replaces the controller and site identifiers.
func (c *Client) SetContextIDs(omadaCID, siteID string) {
	c.contextMu.Lock()
	c.omadaCID = omadaCID
	c.siteID = siteID
	c.contextMu.Unlock()
}

// createHttpClient builds the shared HTTP client with TLS and timeout settings.
func createHttpClient(insecure bool, timeout int) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to init cookiejar: %w", err)
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
	client.SetContextIDs(cid, "")

	sid, err := client.getSiteId(c.Site)
	if err != nil {
		return nil, err
	}
	client.SetContextIDs(cid, *sid)

	client.ControllerKind = client.detectControllerKind()
	if err := client.configureOpenAPIAuth(); err != nil {
		return nil, err
	}

	return client, nil
}

// makeRequest executes an HTTP request with the configured client.
func (c *Client) makeRequest(req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "omada_exporter")

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

	body, err := readBodyWithLimit(resp.Body, maxAPIResponseBodyBytes)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return body, nil
}

// ReadResponseBody reads a successful Omada response with a bounded memory
// footprint. HTTP errors include Omada's structured message when available,
// without echoing arbitrary response bodies that may contain sensitive data.
func ReadResponseBody(resp *http.Response, endpointName string) ([]byte, error) {
	return readResponseBody(resp, endpointName, maxAPIResponseBodyBytes)
}

func readResponseBody(resp *http.Response, endpointName string, limit int64) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("%s returned no HTTP response", endpointName)
	}
	if resp.Body == nil {
		return nil, fmt.Errorf("%s returned an empty HTTP body", endpointName)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("%s response body exceeds %d bytes", endpointName, limit)
	}

	body, err := readBodyWithLimit(resp.Body, limit)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", endpointName, err)
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return body, nil
	}

	status := resp.Status
	if status == "" {
		status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	var apiErr apiErrorResponse
	if json.Unmarshal(body, &apiErr) == nil {
		message := strings.TrimSpace(firstNonEmpty(apiErr.Msg, apiErr.ErrorMsg))
		if code, ok := apiErr.errorCode(); ok {
			if message != "" {
				return nil, fmt.Errorf("%s returned HTTP %s: errorCode %d: %s", endpointName, status, code, message)
			}
			return nil, fmt.Errorf("%s returned HTTP %s: errorCode %d", endpointName, status, code)
		}
		if message != "" {
			return nil, fmt.Errorf("%s returned HTTP %s: %s", endpointName, status, message)
		}
	}
	return nil, fmt.Errorf("%s returned HTTP %s", endpointName, status)
}

func readBodyWithLimit(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
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

func normalizeOption(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func (c *Client) detectControllerKind() string {
	requested := normalizeOption(c.Config.SystemType, config.SystemTypeAuto)
	switch requested {
	case config.SystemTypeFusion, config.SystemTypeStandard:
		return requested
	}

	status, err := c.getControllerSystemStatus()
	if err != nil {
		log.Warn().Err(err).Msg("failed to detect Omada controller kind, assuming standard")
		return config.SystemTypeStandard
	}

	model := strings.ToLower(status.Model + " " + status.ModelName + " " + status.Category)
	if strings.Contains(model, "fusion") {
		return config.SystemTypeFusion
	}
	return config.SystemTypeStandard
}

func (c *Client) configureOpenAPIAuth() error {
	requested := normalizeOption(c.Config.OpenAPIAuth, config.OpenAPIAuthAuto)

	switch requested {
	case config.OpenAPIAuthDisabled:
		c.OpenAPIAuthMode = config.OpenAPIAuthDisabled
		return nil
	case config.OpenAPIAuthWebSession:
		c.OpenAPIAuthMode = config.OpenAPIAuthWebSession
		return c.ensureLoggedIn()
	case config.OpenAPIAuthClientCredentials:
		c.OpenAPIAuthMode = config.OpenAPIAuthClientCredentials
		return c.ensureOpenAPICredentialsConfigured()
	case config.OpenAPIAuthAuto:
	default:
		return fmt.Errorf("unsupported OpenAPI auth mode %q", c.Config.OpenAPIAuth)
	}

	if c.Config.ClientId != "" || c.Config.SecretId != "" {
		c.OpenAPIAuthMode = config.OpenAPIAuthClientCredentials
		return c.ensureOpenAPICredentialsConfigured()
	}

	if c.ControllerKind == config.SystemTypeFusion {
		c.OpenAPIAuthMode = config.OpenAPIAuthWebSession
		return c.ensureLoggedIn()
	}

	c.OpenAPIAuthMode = config.OpenAPIAuthDisabled
	log.Warn().Msg("OpenAPI credentials are not configured; OpenAPI-backed metrics will be skipped")
	return nil
}

func (c *Client) ensureOpenAPICredentialsConfigured() error {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return fmt.Errorf("ClientId and SecretId are required for OpenAPI client-credentials authentication")
	}
	return c.LoginOpenApi()
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

	if urlErr, ok := errors.AsType[*url.Error](err); ok {
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

func (r apiErrorResponse) errorCode() (int, bool) {
	if r.ErrorCodeSnake != nil {
		return *r.ErrorCodeSnake, true
	}
	if r.ErrorCode != nil {
		return *r.ErrorCode, true
	}
	return 0, false
}

// ValidateAPIResponse returns an error when Omada wraps a failed request in an
// otherwise successful HTTP response.
func ValidateAPIResponse(body []byte, endpointName string) error {
	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return nil
	}

	code, hasCode := apiErr.errorCode()
	if !hasCode || code == 0 {
		return nil
	}

	message := strings.TrimSpace(firstNonEmpty(apiErr.Msg, apiErr.ErrorMsg))
	if message == "" {
		message = "Omada API error"
	}
	return fmt.Errorf("%s returned errorCode %d: %s", endpointName, code, message)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
	contextual, err := c.requestWithCurrentContext(req)
	if err != nil {
		return nil, err
	}
	cloned, err := cloneRequest(contextual)
	if err != nil {
		return nil, err
	}
	return c.makeRequest(cloned)
}

// requestWithCurrentContext refreshes session-scoped IDs immediately before a
// request is sent. A request can be built before another goroutine completes
// reauthentication, so replaying its original URL or body verbatim may keep
// using stale controller or site IDs.
func (c *Client) requestWithCurrentContext(req *http.Request) (*http.Request, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("request URL is nil")
	}

	omadaCID, siteID := c.ContextIDs()
	contextual := req.Clone(req.Context())
	parts := strings.Split(contextual.URL.Path, "/")
	cidIndex, apiStart := -1, -1
	for i := 0; i+1 < len(parts); i++ {
		switch {
		case parts[i] == "api" && parts[i+1] == "v2" && i > 0:
			cidIndex, apiStart = i-1, i+2
		case parts[i] == "openapi" && (parts[i+1] == "v1" || parts[i+1] == "v2") && i+2 < len(parts):
			cidIndex, apiStart = i+2, i+3
		}
	}
	if cidIndex >= 0 && omadaCID != "" {
		parts[cidIndex] = omadaCID
	}
	if siteID != "" {
		for i := apiStart; i >= 0 && i+2 < len(parts); i++ {
			if parts[i] == "sites" {
				parts[i+1] = siteID
				break
			}
		}
	}
	contextual.URL.Path = strings.Join(parts, "/")
	contextual.URL.RawPath = ""

	if contextual.Method == http.MethodPost && strings.HasSuffix(contextual.URL.Path, "/api/v2/sites/alert-count") {
		body, err := json.Marshal(struct {
			SiteIDs []string `json:"siteIds"`
		}{SiteIDs: []string{siteID}})
		if err != nil {
			return nil, fmt.Errorf("encode refreshed alert request: %w", err)
		}
		contextual.Body = io.NopCloser(bytes.NewReader(body))
		contextual.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		contextual.ContentLength = int64(len(body))
	}

	return contextual, nil
}

// ensureLoggedIn makes sure the web session is authenticated.
func (c *Client) ensureLoggedIn() error {
	// Only one goroutine should check/login at a time. Without singleflight,
	// a Prometheus scrape that touches several web API collectors can trigger
	// several identical login attempts when the session expires.
	_, err, _ := c.webAuthGroup.Do("web-auth", func() (any, error) {
		unlock := c.webAuthTransition.lock()
		defer unlock()

		loggedIn, err := c.IsLoggedIn()
		if err != nil {
			return nil, err
		}
		if !loggedIn {
			log.Info().Str("user", c.Config.Username).Msg("not logged in, logging in")
			omadaCID, _ := c.ContextIDs()
			if err := c.login(omadaCID); err != nil || c.currentWebToken() == "" {
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
		unlock := c.webAuthTransition.lock()
		defer unlock()

		omadaCID, err := c.getCid()
		if err != nil {
			return nil, err
		}
		if err := c.login(omadaCID); err != nil {
			return nil, err
		}

		siteID, err := c.getSiteIdFromCurrentSessionForCID(c.Config.Site, omadaCID)
		if err != nil {
			return nil, err
		}
		c.SetContextIDs(omadaCID, *siteID)
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
		return nil, err
	}

	authFailed, err := isWebAPIAuthFailure(resp)
	if err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	if !authFailed {
		return resp, nil
	}

	// Omada sometimes returns a JSON auth error with HTTP 200. We inspect the
	// body, close the failed response, refresh the session, and replay the
	// request once with current session-scoped IDs.
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
		_ = resp.Body.Close()
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
	contextual, err := c.requestWithCurrentContext(req)
	if err != nil {
		return nil, err
	}
	cloned, err := cloneRequest(contextual)
	if err != nil {
		return nil, err
	}

	cloned.Header.Set("Accept", "application/json")
	cloned.Header.Set("X-Requested-With", "XMLHttpRequest")
	cloned.Header.Set("User-Agent", "omada_exporter")
	if c.OpenAPIAuthMode == config.OpenAPIAuthWebSession {
		cloned.Header.Set("Csrf-Token", c.currentWebToken())
		cloned.Header.Set("Omada-Request-Source", "web-local")
	} else {
		cloned.Header.Set("Authorization", "AccessToken="+c.currentOpenAPIAccessToken())
	}

	resp, err := c.httpClient.Do(cloned)
	if err != nil {
		return nil, redactError(err)
	}
	return resp, nil
}

// ensureOpenAPIAccessToken makes sure the Open API token is available.
func (c *Client) ensureOpenAPIAccessToken() error {
	switch c.OpenAPIAuthMode {
	case config.OpenAPIAuthWebSession:
		return c.ensureLoggedIn()
	case config.OpenAPIAuthDisabled:
		return fmt.Errorf("OpenAPI authentication is disabled or unavailable")
	case config.OpenAPIAuthClientCredentials, "":
	default:
		return fmt.Errorf("unsupported OpenAPI auth mode %q", c.OpenAPIAuthMode)
	}

	// The Open API uses access/refresh tokens instead of the web CSRF token.
	// The refresh/login work is deduplicated for the same reason as web login:
	// a single scrape can need several Open API collectors at the same time.
	_, err, _ := c.openAPIAuthGroup.Do("openapi-auth", func() (any, error) {
		unlock := c.openAuthTransition.lock()
		defer unlock()

		accessToken, refreshToken, expiresAt := c.currentOpenAPITokenState()
		now := time.Now()
		if now.After(expiresAt) && refreshToken != "" {
			if err := c.refreshOpenAPIToken(); err != nil {
				log.Warn().Err(err).Msg("failed to refresh OpenAPI token, requesting a new one")
				c.clearOpenAPITokens()
			}
		}

		accessToken, _, expiresAt = c.currentOpenAPITokenState()
		if expiresAt.IsZero() || time.Now().After(expiresAt) || accessToken == "" {
			omadaCID, _ := c.ContextIDs()
			return nil, c.loginOpenAPI(omadaCID)
		}

		return nil, nil
	})
	return err
}

// reauthenticateOpenAPISession refreshes the Open API session after authentication expires.
func (c *Client) reauthenticateOpenAPISession() error {
	if c.OpenAPIAuthMode == config.OpenAPIAuthWebSession {
		return c.reauthenticateWebSession()
	}
	if c.OpenAPIAuthMode == config.OpenAPIAuthDisabled {
		return fmt.Errorf("OpenAPI authentication is disabled or unavailable")
	}

	_, err, _ := c.openAPIAuthGroup.Do("openapi-reauth", func() (any, error) {
		unlock := c.openAuthTransition.lock()
		defer unlock()

		omadaCID, err := c.getCid()
		if err != nil {
			return nil, err
		}

		c.clearOpenAPITokens()
		if err := c.loginOpenAPI(omadaCID); err != nil {
			return nil, err
		}
		_, siteID := c.ContextIDs()
		c.SetContextIDs(omadaCID, siteID)
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
		return nil, err
	}

	authFailed, err := isOpenAPIAuthFailure(resp)
	if err != nil {
		_ = resp.Body.Close()
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
		_ = resp.Body.Close()
		return nil, err
	}
	if authFailed {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request remained unauthorized after re-authentication")
	}

	return resp, nil
}
