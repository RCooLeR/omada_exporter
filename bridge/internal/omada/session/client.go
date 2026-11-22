package session

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"golang.org/x/sync/singleflight"
)

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

	// MSP mode. The root client authenticates with the MSP ID (web login,
	// loginStatus, OpenAPI token) and owns all credential state. Target views
	// created by ForTarget carry only a customer/site path context and a cache,
	// and delegate every authentication operation to authParent, so one web
	// session and one OpenAPI token serve all customers. authParent is nil for
	// the root client and in non-MSP mode.
	authParent        *Client
	mspMode           bool
	lastWebAuthAt     time.Time // guarded by authMu
	lastOpenAPIAuthAt time.Time // guarded by authMu
	requestsOK        atomic.Uint64
	requestsFailed    atomic.Uint64
}

// authRoot returns the client that owns authentication state.
func (c *Client) authRoot() *Client {
	if c.authParent != nil {
		return c.authParent
	}
	return c
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

// Settings returns the shared exporter configuration. Callers must treat it as read-only
// after startup; it is not a snapshot and may contain credentials.
func (c *Client) Settings() *config.Config {
	return c.Config
}

// currentWebToken returns the CSRF token guarded by authMu.
//
// A single exporter scrape can touch several collectors at once. Those
// collectors share one API client, so credential fields must be read and
// written under a lock to keep retries and background MQTT publishing from
// racing each other.
func (c *Client) currentWebToken() string {
	c = c.authRoot()
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.token
}

func (c *Client) setWebToken(token string) {
	c = c.authRoot()
	c.authMu.Lock()
	c.token = token
	c.lastWebAuthAt = time.Now()
	c.authMu.Unlock()
}

func (c *Client) currentOpenAPITokenState() (string, string, time.Time) {
	c = c.authRoot()
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.accessToken, c.refreshToken, c.accessTokenExpiresAt
}

func (c *Client) setOpenAPITokens(accessToken, refreshToken string, expiresIn int32) {
	c = c.authRoot()
	lifetime := time.Duration(expiresIn) * time.Second
	if lifetime > 5*time.Second {
		lifetime -= 5 * time.Second
	}

	c.authMu.Lock()
	c.accessToken = accessToken
	c.refreshToken = refreshToken
	c.accessTokenExpiresAt = time.Now().Add(lifetime)
	c.lastOpenAPIAuthAt = time.Now()
	c.authMu.Unlock()
}

func (c *Client) clearOpenAPITokens() {
	c = c.authRoot()
	c.authMu.Lock()
	c.accessToken = ""
	c.refreshToken = ""
	c.accessTokenExpiresAt = time.Time{}
	c.authMu.Unlock()
}

func (c *Client) currentOpenAPIAccessToken() string {
	c = c.authRoot()
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.accessToken
}
