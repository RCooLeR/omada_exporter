package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
)

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
