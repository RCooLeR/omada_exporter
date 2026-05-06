package model

import (
	"strconv"
	"strings"
)

// vpnModeString converts a VPN mode code into its label.
func vpnModeString(mode int8) string {
	switch mode {
	case 0:
		return "Server"
	case 1:
		return "Client"
	}
	return ""
}

// vpnTypeString converts a VPN type code into its label.
func vpnTypeString(vpnType int8) string {
	switch vpnType {
	case 0:
		return "L2TP"
	case 1:
		return "PPTP"
	case 2:
		return "IPSec"
	case 3:
		return "OpenVPN"
	case 4:
		return "WireGuard"
	case 5:
		return "SSL VPN"
	}
	return ""
}

// siteToSiteVpnTypeString converts a site-to-site VPN type code into its label.
func siteToSiteVpnTypeString(siteVpnType int8) string {
	switch siteVpnType {
	case 0:
		return "Auto"
	case 1:
		return "Manual"
	}
	return ""
}

// parseUptimeSeconds parses an uptime string into seconds.
func parseUptimeSeconds(uptime string) int64 {
	var totalSeconds int64

	for _, part := range strings.Fields(uptime) {
		value, unit, ok := splitNumericSuffix(part)
		if !ok {
			continue
		}

		switch unit {
		case "d":
			totalSeconds += int64(value) * 24 * 60 * 60
		case "h":
			totalSeconds += int64(value) * 60 * 60
		case "m":
			totalSeconds += int64(value) * 60
		case "s":
			totalSeconds += int64(value)
		}
	}

	return totalSeconds
}

// splitNumericSuffix separates a trailing numeric value from its unit suffix.
func splitNumericSuffix(part string) (int, string, bool) {
	if len(part) < 2 {
		return 0, "", false
	}

	unit := part[len(part)-1:]
	value, err := strconv.Atoi(part[:len(part)-1])
	if err != nil {
		return 0, "", false
	}

	return value, unit, true
}
