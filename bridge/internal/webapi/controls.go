package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/RCooLeR/omada_exporter/internal/api"
)

const (
	ControlActionRebootDevice        = "reboot_device"
	ControlActionReconnectWiFiClient = "reconnect_client"
	controlClientPageSize            = 100
	controlMaxClientPages            = 1000
)

// ControlTarget is an eligible target in the configured controller/site only.
type ControlTarget struct {
	SiteID     string
	MAC        string
	Name       string
	DeviceType string
	Model      string
	Action     string
}

var controlMACPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{12}|(?:[0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}|(?:[0-9a-fA-F]{2}-){5}[0-9a-fA-F]{2})$`)

// CanonicalControlMAC accepts only an exact six-byte MAC (bare, colon or hyphen
// notation) and returns Omada's uppercase hyphen notation. No path/space input
// is silently normalized into an executable target.
func CanonicalControlMAC(value string) (string, error) {
	if !controlMACPattern.MatchString(value) {
		return "", fmt.Errorf("invalid control MAC address")
	}
	mac := strings.ToUpper(strings.NewReplacer(":", "", "-", "").Replace(value))
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = mac[i*2 : i*2+2]
	}
	return strings.Join(parts, "-"), nil
}

// ControlTargets reads current controller inventory, not telemetry caches.
func (c *Client) ControlTargets(ctx context.Context) ([]ControlTarget, error) {
	_, siteID := c.ContextIDs()
	var targets []ControlTarget
	err := c.WithWebControlSession(ctx, siteID, func(session *api.WebControlSession) error {
		devices, err := controlDevices(session)
		if err != nil {
			return err
		}
		clients, err := controlClients(session)
		if err != nil {
			return err
		}
		targets = append(devices, clients...)
		return nil
	})
	return targets, err
}

// RebootDevice restarts an adopted online AP, standalone switch or gateway.
// Controller/OLT/stack reboot APIs deliberately are not exposed by this action.
// The route and {mac} body match Omada's device-index reboot handler; the
// reconnect route below matches its client API helper (an empty POST body).
// Both were checked against Fusion 6.3.14.1 and standard 6.3.0.45 frontends.
func (c *Client) RebootDevice(ctx context.Context, siteID, mac string) error {
	return c.control(ctx, siteID, mac, ControlActionRebootDevice)
}

// ReconnectWiFiClient disconnects an active wireless association; the station
// decides whether/when to associate again. It does not reboot or block it.
func (c *Client) ReconnectWiFiClient(ctx context.Context, siteID, mac string) error {
	return c.control(ctx, siteID, mac, ControlActionReconnectWiFiClient)
}

func (c *Client) control(ctx context.Context, siteID, mac, action string) error {
	canonical, err := CanonicalControlMAC(mac)
	if err != nil {
		return err
	}
	return c.WithWebControlSession(ctx, siteID, func(session *api.WebControlSession) error {
		var targets []ControlTarget
		var err error
		if action == ControlActionRebootDevice {
			targets, err = controlDevices(session)
		} else {
			targets, err = controlClients(session)
		}
		if err != nil {
			return err
		}
		matches := 0
		for _, target := range targets {
			if target.MAC == canonical && target.Action == action {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("control target is missing, ambiguous, offline or unsupported")
		}
		if action == ControlActionRebootDevice {
			return session.PostJSON("/cmd/devices/"+canonical+"/reboot", struct {
				MAC string `json:"mac"`
			}{canonical})
		}
		return session.PostJSON("/cmd/clients/"+canonical+"/reconnect", nil)
	})
}

func controlDevices(session *api.WebControlSession) ([]ControlTarget, error) {
	var response struct {
		Result []struct {
			MAC      string          `json:"mac"`
			Name     string          `json:"name"`
			Type     string          `json:"type"`
			Model    string          `json:"model"`
			SiteID   string          `json:"siteId"`
			Status   int             `json:"status"`
			StackMsg json.RawMessage `json:"stackMsg"`
		} `json:"result"`
	}
	if err := session.ReadJSON("/devices", &response); err != nil {
		return nil, err
	}
	var targets []ControlTarget
	seen := map[string]bool{}
	for _, device := range response.Result {
		if device.Type != "ap" && device.Type != "switch" && device.Type != "gateway" {
			continue
		}
		if device.Status != 14 && device.Status != 15 {
			continue
		}
		if device.SiteID != "" && device.SiteID != session.SiteID {
			continue
		}
		if len(device.StackMsg) > 0 && string(device.StackMsg) != "null" {
			continue
		}
		mac, err := CanonicalControlMAC(device.MAC)
		if err != nil {
			continue
		}
		if seen[mac] {
			return nil, fmt.Errorf("duplicate control device in fresh inventory")
		}
		seen[mac] = true
		targets = append(targets, ControlTarget{session.SiteID, mac, device.Name, device.Type, device.Model, ControlActionRebootDevice})
	}
	return targets, nil
}

func controlClients(session *api.WebControlSession) ([]ControlTarget, error) {
	var targets []ControlTarget
	seen := map[string]bool{}
	rowsRead := 0
	for page := 1; page <= controlMaxClientPages; page++ {
		var response struct {
			Result struct {
				TotalRows   int `json:"totalRows"`
				CurrentPage int `json:"currentPage"`
				Data        []struct {
					MAC        string `json:"mac"`
					Name       string `json:"name"`
					Model      string `json:"model"`
					SiteID     string `json:"siteId"`
					Wireless   bool   `json:"wireless"`
					Active     bool   `json:"active"`
					Blocked    bool   `json:"blocked"`
					AuthStatus *int   `json:"authStatus"`
				} `json:"data"`
			} `json:"result"`
		}
		// The Web API uses different pagination keys than the OpenAPI. Unknown
		// keys silently return its default first page of only ten clients.
		endpoint := fmt.Sprintf("/clients?currentPage=%d&currentPageSize=%d&filters.active=true", page, controlClientPageSize)
		if err := session.ReadJSON(endpoint, &response); err != nil {
			return nil, err
		}
		if response.Result.CurrentPage != 0 && response.Result.CurrentPage != page {
			return nil, fmt.Errorf("control client inventory returned unexpected page")
		}
		if len(response.Result.Data) == 0 && rowsRead < response.Result.TotalRows {
			return nil, fmt.Errorf("control client inventory ended before all rows were read")
		}
		// Count all returned rows, including ineligible clients. Controllers may
		// cap the requested page size, so page*controlClientPageSize is unsafe.
		rowsRead += len(response.Result.Data)
		for _, client := range response.Result.Data {
			if !client.Wireless || !client.Active || client.Blocked || client.AuthStatus == nil || *client.AuthStatus < 0 || *client.AuthStatus > 3 {
				continue
			}
			if client.SiteID != "" && client.SiteID != session.SiteID {
				continue
			}
			mac, err := CanonicalControlMAC(client.MAC)
			if err != nil {
				continue
			}
			if seen[mac] {
				return nil, fmt.Errorf("duplicate control client in fresh inventory")
			}
			seen[mac] = true
			targets = append(targets, ControlTarget{session.SiteID, mac, client.Name, "client", client.Model, ControlActionReconnectWiFiClient})
		}
		if len(response.Result.Data) == 0 || response.Result.TotalRows <= rowsRead {
			return targets, nil
		}
	}
	return nil, fmt.Errorf("control client inventory exceeds pagination limit")
}
