package session

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
)

// mspReauthMinInterval bounds how often a target view may force the shared MSP
// session to log in again. Some per-customer API errors (for example -1005
// "Operation forbidden") look like authentication failures; without a bound a
// single misconfigured customer would replace the shared credentials on every
// request and disturb every other customer.
var mspReauthMinInterval = 30 * time.Second

type authKind int

const (
	webAuthKind authKind = iota
	openAPIAuthKind
)

// useMSPMode resolves whether the exporter runs in MSP multi-customer mode.
// OMADA_SYSTEM_TYPE=auto follows the controller's /api/info mspMode flag;
// OMADA_SYSTEM_TYPE=msp forces it. An explicit standard/fusion keeps the
// single-site behaviour even on an MSP controller.
func useMSPMode(systemType string, reported bool) bool {
	switch requested := normalizeOption(systemType, config.SystemTypeAuto); requested {
	case config.SystemTypeMSP:
		if !reported {
			log.Warn().Msg("OMADA_SYSTEM_TYPE=msp is set but the controller does not report mspMode; continuing in MSP mode")
		}
		return true
	case config.SystemTypeAuto:
		return reported
	default:
		if reported {
			log.Warn().Str("system_type", requested).Msg("controller reports MSP mode but OMADA_SYSTEM_TYPE forces single-site mode; site lookup is expected to fail. Use OMADA_SYSTEM_TYPE=auto or msp")
		}
		return false
	}
}

// configureMSP authenticates the root client with the MSP ID. The context of
// the root client stays (mspId, "") for its whole life; customer and site IDs
// only ever live on target views.
func (c *Client) configureMSP() error {
	if err := config.ValidateMSP(c.Config); err != nil {
		return err
	}
	c.mspMode = true
	c.ControllerKind = config.SystemTypeMSP
	c.OpenAPIAuthMode = config.OpenAPIAuthClientCredentials

	mspID, _ := c.ContextIDs()
	log.Info().Str("msp_id", mspID).Msg("Omada controller is in MSP mode; authenticating with the MSP ID")
	if err := c.ensureLoggedIn(); err != nil {
		return fmt.Errorf("MSP web login: %w", err)
	}
	if err := c.LoginOpenApi(); err != nil {
		return fmt.Errorf("MSP OpenAPI login (the Open API application must be created at MSP level in Client mode): %w", err)
	}
	return nil
}

// MSPMode reports whether the client talks to an Omada controller in MSP mode.
func (c *Client) MSPMode() bool {
	return c != nil && c.authRoot().mspMode
}

// MSPID returns the MSP ID used for authentication, or "" outside MSP mode.
func (c *Client) MSPID() string {
	if !c.MSPMode() {
		return ""
	}
	id, _ := c.authRoot().ContextIDs()
	return id
}

// ForTarget returns a lightweight view for one MSP customer and site. The view
// puts customerID where the controller ID goes in request paths and shares the
// root's HTTP client, cookie jar, CSRF token and OpenAPI token. It has its own
// request cache and a private copy of the configuration whose Site is the
// target's site name, so existing collectors label metrics correctly.
//
// siteID may be empty for customer-scoped requests such as site discovery.
func (c *Client) ForTarget(customerID, siteID, siteName string) (*Client, error) {
	root := c.authRoot()
	if !root.mspMode {
		return nil, fmt.Errorf("target views are only available in MSP mode")
	}
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, fmt.Errorf("target view requires a customer ID")
	}
	conf := *root.Config
	conf.Site = siteName
	view := &Client{
		Config:          &conf,
		ControllerKind:  root.ControllerKind,
		OpenAPIAuthMode: root.OpenAPIAuthMode,
		requestCache:    map[string]cacheEntry{},
		authParent:      root,
		mspMode:         true,
	}
	view.SetContextIDs(customerID, siteID)
	return view, nil
}

// RequestStats returns how many API requests made through this target view
// succeeded and failed. A request fails on a transport error, an HTTP error
// status, or a non-zero Omada errorCode. Only target views count requests.
func (c *Client) RequestStats() (ok, failed uint64) {
	return c.requestsOK.Load(), c.requestsFailed.Load()
}

func (c *Client) recordOutcome(resp *http.Response, err error) {
	if c.authParent == nil {
		return
	}
	if err != nil || resp == nil || resp.StatusCode >= http.StatusBadRequest {
		c.requestsFailed.Add(1)
		return
	}
	body, readErr := readAndRestoreBody(resp)
	if readErr != nil || ValidateAPIResponse(body, "") != nil {
		c.requestsFailed.Add(1)
		return
	}
	c.requestsOK.Add(1)
}

// reauthenticateForTarget runs on the root when a target view saw an
// authentication failure. It re-logs in with the MSP ID unless the shared
// credentials were renewed very recently; the caller then replays its request
// with its own (unchanged) customer and site IDs.
func (c *Client) reauthenticateForTarget(kind authKind) error {
	c.authMu.RLock()
	last := c.lastWebAuthAt
	if kind == openAPIAuthKind {
		last = c.lastOpenAPIAuthAt
	}
	c.authMu.RUnlock()
	if !last.IsZero() && time.Since(last) < mspReauthMinInterval {
		log.Debug().Msg("shared MSP credentials were renewed recently; retrying without a new login")
		return nil
	}
	if kind == openAPIAuthKind {
		return c.reauthenticateOpenAPISession()
	}
	return c.reauthenticateWebSession()
}

// reauthenticateMSPWebSession logs in again with the MSP ID. Unlike the
// single-site path it does not resolve a site: /{mspId}/api/v2/users/current
// fails on MSP controllers, and the root client has no site.
func (c *Client) reauthenticateMSPWebSession() error {
	_, err, _ := c.webAuthGroup.Do("web-reauth", func() (any, error) {
		unlock := c.webAuthTransition.lock()
		defer unlock()

		mspID, err := c.getCid()
		if err != nil {
			return nil, err
		}
		if err := c.login(mspID); err != nil {
			return nil, err
		}
		c.SetContextIDs(mspID, "")
		c.invalidateRequestCache()
		return nil, nil
	})
	return err
}
