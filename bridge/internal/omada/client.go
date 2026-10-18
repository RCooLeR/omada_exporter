package omada

import (
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

// Client provides controller data and guarded controls independently of which
// Omada endpoint family supplies them. All features share one authenticated
// session and its cache; transport and authentication are not exposed to callers.
type Client struct {
	session *session.Client
}

// NewClient composes feature operations over an existing authenticated session.
// A nil session returns nil so metric descriptors can be built without a controller.
func NewClient(shared *session.Client) *Client {
	if shared == nil {
		return nil
	}
	return &Client{session: shared}
}

// Settings returns the shared exporter configuration, or nil without a session.
// Callers must treat it as read-only after startup; it may contain credentials.
func (c *Client) Settings() *config.Config {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.Settings()
}

// ContextIDs returns the current controller and site identifiers as an atomic pair.
// They are read from the shared session because authentication can replace them.
func (c *Client) ContextIDs() (string, string) {
	if c == nil || c.session == nil {
		return "", ""
	}
	return c.session.ContextIDs()
}

// OpenAPIEnabled reports whether the resolved session supports OpenAPI telemetry.
// An absent session returns true for descriptor-only collector construction.
func (c *Client) OpenAPIEnabled() bool {
	return c == nil || c.session == nil || c.session.OpenAPIAuthMode != config.OpenAPIAuthDisabled
}
