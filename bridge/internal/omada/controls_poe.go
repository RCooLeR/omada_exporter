package omada

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

// PoEControlTargets reads optional, uncached PoE inventory. Unsupported or
// unreadable owners produce no buttons, without disabling reboot/reconnect.
func (c *Client) PoEControlTargets(ctx context.Context) ([]ControlTarget, error) {
	_, siteID := c.ContextIDs()
	var targets []ControlTarget
	err := c.session.WithCurrentWebControlSession(ctx, siteID, func(s *session.WebControlSession) error {
		devices, err := controlDevices(s)
		if err != nil {
			return err
		}
		for _, owner := range devices {
			if owner.DeviceType != "switch" && owner.DeviceType != "gateway" {
				continue
			}
			ports, err := controlPoEPorts(s, owner)
			if err != nil {
				// Optional/version-specific reads fail closed for this owner only.
				// Commands repeat the same reads and return their errors to callers.
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}
			targets = append(targets, ports...)
		}
		// Binding discovery is optional. Never guess from client/switch telemetry
		// when this controller lacks IntelliRecover's eligibility contract.
		if err := bindPoEEndpoints(s, targets); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	})
	return targets, err
}

// PowerCyclePoEPort submits one native PoE restart, never an off/on sequence.
// The owner and physical port are freshly validated in a pinned site/session.
// A supplied powered-device binding must still match; an unknown binding cannot
// be substituted for a previously identified device.
func (c *Client) PowerCyclePoEPort(ctx context.Context, siteID, ownerMAC string, port int, expectedPoweredDeviceMAC string) error {
	mac, err := CanonicalControlMAC(ownerMAC)
	if err != nil {
		return err
	}
	if port < 1 || port > 255 {
		return fmt.Errorf("invalid PoE port number")
	}
	if expectedPoweredDeviceMAC != "" {
		expectedPoweredDeviceMAC, err = CanonicalControlMAC(expectedPoweredDeviceMAC)
		if err != nil {
			return err
		}
	}
	return c.session.WithWebControlSession(ctx, siteID, func(s *session.WebControlSession) error {
		devices, err := controlDevices(s)
		if err != nil {
			return err
		}
		for _, owner := range devices {
			if owner.MAC != mac || (owner.DeviceType != "switch" && owner.DeviceType != "gateway") {
				continue
			}
			targets, err := controlPoEPorts(s, owner)
			if err != nil {
				return err
			}
			if err := bindPoEEndpoints(s, targets); err != nil {
				if expectedPoweredDeviceMAC != "" || ctx.Err() != nil {
					return fmt.Errorf("revalidate PoE powered endpoint: %w", err)
				}
			}
			for _, target := range targets {
				if target.Port != port || target.PoweredDeviceMAC != expectedPoweredDeviceMAC {
					continue
				}
				// Verified against Omada's native port restart handlers. These POSTs
				// have no body and are never retried, including unknown outcomes.
				path := "/cmd/switches/" + mac
				if owner.DeviceType == "gateway" {
					path = "/gateways/" + mac
				}
				return s.PostJSON(path+"/ports/"+strconv.Itoa(port)+"/restart", nil)
			}
		}
		return fmt.Errorf("PoE target is missing, changed, ambiguous or unsupported")
	})
}

type controlPoEPort struct {
	Port        int    `json:"port"`
	Name        string `json:"name"`
	Operation   string `json:"operation"`
	ConfigStack bool   `json:"configStack"`
	MADUsed     bool   `json:"madUsed"`
	MLAGPeer    bool   `json:"configMlagPeerLink"`
	MLAGDAD     bool   `json:"configMlagDad"`
	SupportPoE  *bool  `json:"supportPoe"`
	PortStatus  struct {
		Port int  `json:"port"`
		PoE  bool `json:"poe"`
	} `json:"portStatus"`
}

func controlPoEPorts(s *session.WebControlSession, owner ControlTarget) ([]ControlTarget, error) {
	if owner.DeviceType == "gateway" {
		return controlGatewayPoEPorts(s, owner)
	}
	var detail struct {
		Result struct {
			MAC    string `json:"mac"`
			SiteID string `json:"siteId"`
			Site   string `json:"site"`
			DevCap struct {
				PoESupport bool `json:"poeSupport"`
			} `json:"devCap"`
			Uplink struct {
				Port int `json:"port"`
			} `json:"uplink"`
		} `json:"result"`
	}
	if err := s.ReadJSON("/switches/"+owner.MAC, &detail); err != nil {
		return nil, err
	}
	mac, err := CanonicalControlMAC(detail.Result.MAC)
	if err != nil || mac != owner.MAC || !detail.Result.DevCap.PoESupport ||
		(detail.Result.SiteID != "" && detail.Result.SiteID != s.SiteID) || (detail.Result.Site != "" && detail.Result.Site != s.SiteID) {
		return nil, fmt.Errorf("PoE switch capability or identity is not confirmed")
	}
	var response struct {
		Result []controlPoEPort `json:"result"`
	}
	if err := s.ReadJSON("/switches/"+owner.MAC+"/ports", &response); err != nil {
		return nil, err
	}
	counts := map[int]int{}
	for _, port := range response.Result {
		counts[port.Port]++
	}
	var targets []ControlTarget
	for _, port := range response.Result {
		if port.Port < 1 || port.Port > 255 || counts[port.Port] != 1 || port.Port == detail.Result.Uplink.Port ||
			port.ConfigStack || port.MADUsed || port.MLAGPeer || port.MLAGDAD ||
			(port.Operation != "switching" && port.Operation != "mirroring") || !port.PortStatus.PoE ||
			(port.SupportPoE != nil && !*port.SupportPoE) || (port.PortStatus.Port != 0 && port.PortStatus.Port != port.Port) {
			continue
		}
		targets = append(targets, poePortTarget(owner, port.Port, port.Name))
	}
	return targets, nil
}

func poePortTarget(owner ControlTarget, port int, name string) ControlTarget {
	owner.Action, owner.Port, owner.PortName = ControlActionPowerCyclePoE, port, name
	return owner
}

func controlGatewayPoEPorts(s *session.WebControlSession, owner ControlTarget) ([]ControlTarget, error) {
	var response struct {
		Result struct {
			MAC              string `json:"mac"`
			SiteID           string `json:"siteId"`
			Site             string `json:"site"`
			SupportPoE       bool   `json:"supportPoe"`
			UnsupportedPorts []int  `json:"unsupportedPorts"`
			PortConfigs      []struct {
				Port           int  `json:"port"`
				PortSupportPoE bool `json:"portSupportPoe"`
				MirrorEnable   bool `json:"mirrorEnable"`
				PortStat       struct {
					Port int    `json:"port"`
					Name string `json:"name"`
					Mode *int   `json:"mode"`
					PoE  bool   `json:"poe"`
				} `json:"portStat"`
			} `json:"portConfigs"`
		} `json:"result"`
	}
	if err := s.ReadJSON("/gateways/"+owner.MAC, &response); err != nil {
		return nil, err
	}
	detail := response.Result
	mac, err := CanonicalControlMAC(detail.MAC)
	if err != nil || mac != owner.MAC || !detail.SupportPoE ||
		(detail.SiteID != "" && detail.SiteID != s.SiteID) || (detail.Site != "" && detail.Site != s.SiteID) {
		return nil, fmt.Errorf("PoE gateway capability or identity is not confirmed")
	}
	counts := map[int]int{}
	for _, port := range detail.PortConfigs {
		counts[port.Port]++
	}
	var targets []ControlTarget
	for _, port := range detail.PortConfigs {
		if port.Port < 1 || port.Port > 255 || counts[port.Port] != 1 || slices.Contains(detail.UnsupportedPorts, port.Port) ||
			!port.PortSupportPoE || port.MirrorEnable || !port.PortStat.PoE || port.PortStat.Port != port.Port ||
			port.PortStat.Mode == nil || *port.PortStat.Mode != 1 {
			continue
		}
		targets = append(targets, poePortTarget(owner, port.Port, port.PortStat.Name))
	}
	return targets, nil
}
