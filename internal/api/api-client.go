package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
)

type Client struct {
	Config               *config.Config
	httpClient           *http.Client
	token                string
	OmadaCID             string
	SiteId               string
	accessToken          string
	refreshToken         string
	accessTokenExpiresAt time.Time
}

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

func Configure(c *config.Config) (*Client, error) {
	httpClient, err := createHttpClient(c.Insecure, c.Timeout)
	if err != nil {
		return nil, err
	}

	client := &Client{
		Config:     c,
		httpClient: httpClient,
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

func (c *Client) makeRequest(req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "omada_exporter")
	req.Header.Set("Connection", "keep-alive")

	if c.token != "" {
		req.Header.Set("Csrf-Token", c.token)
	}

	return c.httpClient.Do(req)
}

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

type apiErrorResponse struct {
	ErrorCode      *int   `json:"errorCode"`
	ErrorCodeSnake *int   `json:"error_code"`
	Msg            string `json:"msg"`
	ErrorMsg       string `json:"errorMsg"`
}

func isHTTPAuthStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

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

func (c *Client) doLoggedInRequest(req *http.Request) (*http.Response, error) {
	cloned, err := cloneRequest(req)
	if err != nil {
		return nil, err
	}
	return c.makeRequest(cloned)
}

func (c *Client) ensureLoggedIn() error {
	loggedIn, err := c.IsLoggedIn()
	if err != nil {
		return err
	}
	if !loggedIn {
		log.Info().Msg(fmt.Sprintf("not logged in, logging in with user: %s", c.Config.Username))
		if err := c.Login(); err != nil || c.token == "" {
			log.Error().Err(err).Msg("failed to login")
			return err
		}
	}
	return nil
}

func (c *Client) reauthenticateWebSession() error {
	if err := c.RefreshOmadaContext(); err != nil {
		return err
	}
	if err := c.Login(); err != nil {
		return err
	}

	siteID, err := c.getSiteIdFromCurrentSession(c.Config.Site)
	if err != nil {
		return err
	}
	c.SiteId = *siteID
	return nil
}

func (c *Client) MakeLoggedInRequest(req *http.Request) (*http.Response, error) {
	if err := c.ensureLoggedIn(); err != nil {
		return nil, err
	}
	log.Info().Msg(fmt.Sprintf("MakeLoggedInRequest %s", req.URL.String()))

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

func (c *Client) doOpenAPIRequest(req *http.Request) (*http.Response, error) {
	cloned, err := cloneRequest(req)
	if err != nil {
		return nil, err
	}

	cloned.Header.Set("Accept", "application/json")
	cloned.Header.Set("X-Requested-With", "XMLHttpRequest")
	cloned.Header.Set("User-Agent", "omada_exporter")
	cloned.Header.Set("Connection", "keep-alive")
	cloned.Header.Set("Authorization", "AccessToken="+c.accessToken)

	return c.httpClient.Do(cloned)
}

func (c *Client) ensureOpenAPIAccessToken() error {
	if time.Now().After(c.accessTokenExpiresAt) && c.refreshToken != "" {
		if err := c.RefreshOpenApiToken(); err != nil {
			log.Warn().Err(err).Msg("failed to refresh OpenAPI token, requesting a new one")
			c.accessToken = ""
			c.refreshToken = ""
			c.accessTokenExpiresAt = time.Time{}
		}
	}

	if c.accessTokenExpiresAt.IsZero() || time.Now().After(c.accessTokenExpiresAt) || c.accessToken == "" {
		return c.LoginOpenApi()
	}

	return nil
}

func (c *Client) reauthenticateOpenAPISession() error {
	if err := c.RefreshOmadaContext(); err != nil {
		return err
	}

	c.accessToken = ""
	c.refreshToken = ""
	c.accessTokenExpiresAt = time.Time{}
	return c.LoginOpenApi()
}

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
