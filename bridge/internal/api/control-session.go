package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// WebControlSession pins a controller, site and web session while a control
// target is freshly validated and its command is sent. It is only valid inside
// WithWebControlSession's callback; callbacks must not call other Client methods.
type WebControlSession struct {
	SiteID  string
	ctx     context.Context
	client  *http.Client
	baseURL string
	token   string
}

// WithWebControlSession prevents reauthentication and context changes between
// target validation and a command. Unlike telemetry requests, commands never
// retry, follow redirects, or rewrite their controller/site after validation.
func (c *Client) WithWebControlSession(ctx context.Context, expectedSiteID string, run func(*WebControlSession) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if expectedSiteID == "" {
		return fmt.Errorf("control requires an explicit site ID")
	}
	cid, siteID := c.ContextIDs()
	if cid == "" || siteID != expectedSiteID {
		return fmt.Errorf("control site does not match the configured site")
	}
	if err := c.ensureLoggedIn(); err != nil {
		return fmt.Errorf("authenticate control session: %w", err)
	}
	unlock, err := c.webAuthTransition.lockControlContext(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	c.contextMu.RLock()
	defer c.contextMu.RUnlock()
	if c.omadaCID != cid || c.siteID != expectedSiteID {
		return fmt.Errorf("controller/site changed before control; refresh and try again")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	session := &WebControlSession{
		SiteID: expectedSiteID, ctx: ctx, client: &client, token: c.currentWebToken(),
		baseURL: strings.TrimRight(c.Config.Host, "/") + "/" + url.PathEscape(cid) + "/api/v2/sites/" + url.PathEscape(expectedSiteID),
	}
	return run(session)
}

func (g *authTransitionGate) lockControlContext(ctx context.Context) (func(), error) {
	g.once.Do(func() { g.permit = make(chan struct{}, 1); g.permit <- struct{}{} })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.permit:
		return func() { g.permit <- struct{}{} }, nil
	}
}

// ReadJSON reads uncached site inventory in this pinned session.
func (s *WebControlSession) ReadJSON(relativePath string, target any) error {
	body, err := s.request(http.MethodGet, relativePath, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

// PostJSON submits one command. A nil payload produces the empty body expected
// by Omada's reconnect operation. Success requires an explicit errorCode: 0.
func (s *WebControlSession) PostJSON(relativePath string, payload any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	_, err = s.request(http.MethodPost, relativePath, body)
	return err
}

func (s *WebControlSession) request(method, relativePath string, payload []byte) ([]byte, error) {
	path, err := url.Parse(relativePath)
	if err != nil || !strings.HasPrefix(relativePath, "/") || strings.HasPrefix(relativePath, "//") || path.IsAbs() || path.Host != "" || path.Fragment != "" || strings.Contains(path.Path, "..") || strings.Contains(path.Path, "\\") {
		return nil, fmt.Errorf("invalid site-relative control endpoint")
	}
	var body io.Reader
	if payload != nil {
		body = io.NopCloser(bytes.NewReader(payload))
	}
	req, err := http.NewRequestWithContext(s.ctx, method, s.baseURL+relativePath, body)
	if err != nil {
		return nil, err
	}
	// No GetBody or idempotency headers: HTTP transport must not replay a command.
	req.GetBody = nil
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "omada_exporter")
	if s.token != "" {
		req.Header.Set("Csrf-Token", s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("control request failed (command outcome may be unknown; not retried): %w", redactError(err))
	}
	defer resp.Body.Close()
	data, err := ReadResponseBody(resp, "control endpoint")
	if err != nil {
		return nil, err
	}
	if err := ValidateAPIResponse(data, "control endpoint"); err != nil {
		return nil, err
	}
	var ack struct {
		ErrorCode *int `json:"errorCode"`
	}
	if err := json.Unmarshal(data, &ack); err != nil {
		return nil, fmt.Errorf("invalid control response: %w", err)
	}
	if ack.ErrorCode == nil || *ack.ErrorCode != 0 {
		return nil, fmt.Errorf("control endpoint did not acknowledge success")
	}
	return data, nil
}
