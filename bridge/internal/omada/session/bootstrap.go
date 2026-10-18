package session

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
)

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
	// This startup-only configuration notice must remain visible at the default
	// error threshold without reporting an expected disabled capability as an error.
	log.Log().Str("event", "openapi_disabled").Msg("OMADA_CLIENT_ID and OMADA_SECRET_ID are not configured; skipping OpenAPI-backed client, ISP, VPN, WAN and network-quality data; Web API collection continues")
	return nil
}

func (c *Client) ensureOpenAPICredentialsConfigured() error {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return fmt.Errorf("ClientId and SecretId are required for OpenAPI client-credentials authentication")
	}
	return c.LoginOpenApi()
}
