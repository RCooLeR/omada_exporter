package model

// VpnStats represents runtime statistics for a VPN tunnel.
type VpnStats struct {
	VpnID         string `json:"vpnId"`
	Name          string `json:"vpnName"`
	InterfaceName string `json:"interfaceName"`
	VpnMode       int8   `json:"serverType"`
	VpnType       int8   `json:"vpnType"`
	LocalIp       string `json:"localIp"`
	RemoteIp      string `json:"remoteIp"`
	Uptime        string `json:"uptime"`
	DownPkts      int64  `json:"downPkts"`
	DownBytes     int64  `json:"downBytes"`
	UpPkts        int64  `json:"upPkts"`
	UpBytes       int64  `json:"upBytes"`
}

// GetVpnMode converts the VPN mode code to a readable role label.
func (v *VpnStats) GetVpnMode() string {
	return vpnModeString(v.VpnMode)
}

// GetVpnType converts the VPN type code to a readable protocol label.
func (v *VpnStats) GetVpnType() string {
	return vpnTypeString(v.VpnType)
}

// GetUptime parses the VPN uptime string and returns the value in seconds.
func (v *VpnStats) GetUptime() int64 {
	return parseUptimeSeconds(v.Uptime)
}
