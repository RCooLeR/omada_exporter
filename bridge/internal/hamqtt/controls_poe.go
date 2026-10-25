package hamqtt

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/omada"
)

// Optional reads must not consume the whole queued-command freshness budget.
const poeControlInventoryTimeout = 5 * time.Second

// PoE inventory and mutation are optional: existing control providers continue
// to offer reboot/reconnect without claiming that they can cycle port power.
type poeControlAPI interface {
	PoEControlTargets(context.Context) ([]omada.ControlTarget, error)
	PowerCyclePoEPort(context.Context, string, string, int, string) error
}

func validPoEControlPort(port int) bool { return port >= 1 && port <= 255 }

func poeControlBinding(target omada.ControlTarget) (string, bool) {
	if (target.DeviceType != "switch" && target.DeviceType != "gateway") || !validPoEControlPort(target.Port) {
		return "", false
	}
	if _, err := omada.CanonicalControlMAC(target.MAC); err != nil {
		return "", false
	}
	if target.PoweredDeviceMAC == "" {
		return "", true
	}
	mac, err := omada.CanonicalControlMAC(target.PoweredDeviceMAC)
	return mac, err == nil
}

// The payload binds a frontend's discovery record to its observed powered
// device. An empty binding is explicit so it cannot authorize a later occupant.
func poeControlPayload(poweredDeviceMAC string) string { return "PRESS:" + poweredDeviceMAC }

func controlTargetLeaf(mac string, port int, poweredDeviceMAC string) string {
	leaf := trackerID(mac)
	if port != 0 {
		binding := "unknown"
		if poweredDeviceMAC != "" {
			binding = trackerID(poweredDeviceMAC)
		}
		leaf += "_p" + strconv.Itoa(port) + "_b" + binding
	}
	return leaf
}

// Binding changes create new HA entities, but cannot reset a physical port's
// cooldown or allow two different occupants to be advertised for one port.
func controlCooldownKey(button controlButton) string {
	if button.Action != omada.ControlActionPowerCyclePoE {
		return button.CommandTopic
	}
	return button.Action + "/" + button.SiteID + "/" + button.MAC + "/" + strconv.Itoa(button.Port)
}

// Retained discovery authorizes only cleanup of exact canonical topic shapes.
// It never creates a live button or a binding eligible for execution.
func parseControlTargetLeaf(action, leaf string) (string, int, string, bool) {
	macLeaf, port, poweredDeviceMAC := leaf, 0, ""
	switch action {
	case controlReboot, controlReconnect:
	case omada.ControlActionPowerCyclePoE:
		var portText string
		var found bool
		macLeaf, portText, found = strings.Cut(leaf, "_p")
		if !found {
			return "", 0, "", false
		}
		var binding string
		portText, binding, found = strings.Cut(portText, "_b")
		if !found {
			return "", 0, "", false
		}
		if binding != "unknown" {
			mac, ok := normalizeClientMAC(binding)
			if !ok || trackerID(mac) != binding {
				return "", 0, "", false
			}
			poweredDeviceMAC, _ = omada.CanonicalControlMAC(mac)
		}
		var err error
		port, err = strconv.Atoi(portText)
		if err != nil || !validPoEControlPort(port) || strconv.Itoa(port) != portText {
			return "", 0, "", false
		}
	default:
		return "", 0, "", false
	}
	mac, ok := normalizeClientMAC(macLeaf)
	if !ok || trackerID(mac) != macLeaf {
		return "", 0, "", false
	}
	return mac, port, poweredDeviceMAC, true
}
