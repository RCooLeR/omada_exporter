package model

import (
	"strconv"
	"strings"
)

type VpnStats struct {
	Name          string `json:"vpnName"`
	InterfaceName string `json:"interfaceName"`
	VpnMode       int8   `json:"serverType"`
	VpnType       int8   `json:"vpnType"`
	LocalIp       string `json:"localIp"`
	RemoteIp      string `json:"remoteIp"`
	Uptime        string `json:"uptime"`
	DownBytes     int64  `json:"downBytes"`
	UpBytes       int64  `json:"upBytes"`
}

func (v *VpnStats) GetVpnMode() string {
	switch v.VpnMode {
	case 0:
		return "Server"
	case 1:
		return "Client"
	}
	return ""
}
func (v *VpnStats) GetVpnType() string {
	switch v.VpnType {
	case 0:
		return "L2TP"
	case 1:
		return "PPTP"
	case 2:
		return "IPSec"
	case 3:

		return "OpenVPN"
	}
	return ""
}

func (v *VpnStats) GetUptime() int {
	totalMinutes := 0
	parts := strings.Fields(v.Uptime)

	for _, part := range parts {
		if strings.HasSuffix(part, "d") {
			days, _ := strconv.Atoi(strings.TrimSuffix(part, "d"))
			totalMinutes += days * 24 * 60
		} else if strings.HasSuffix(part, "h") {
			hours, _ := strconv.Atoi(strings.TrimSuffix(part, "h"))
			totalMinutes += hours * 60
		} else if strings.HasSuffix(part, "m") {
			minutes, _ := strconv.Atoi(strings.TrimSuffix(part, "m"))
			totalMinutes += minutes
		}
	}

	return totalMinutes
}
