package model

// VpnStats represents runtime statistics for a VPN tunnel.
type VpnStats struct {
	VpnID             string     `json:"vpnId"`
	Name              string     `json:"vpnName"`
	InterfaceName     string     `json:"interfaceName"`
	VpnMode           int8       `json:"serverType"`
	VpnType           int8       `json:"vpnType"`
	LocalIp           string     `json:"localIp"`
	RemoteIp          string     `json:"remoteIp"`
	LocalNetwork      StringList `json:"localNetwork"`
	LocalNetworks     StringList `json:"localNetworks"`
	LocalSubnet       StringList `json:"localSubnet"`
	LocalSubnets      StringList `json:"localSubnets"`
	RemoteNetwork     StringList `json:"remoteNetwork"`
	RemoteNetworks    StringList `json:"remoteNetworks"`
	RemoteSubnet      StringList `json:"remoteSubnet"`
	RemoteSubnets     StringList `json:"remoteSubnets"`
	AllowedIps        StringList `json:"allowedIps"`
	AllowedIPs        StringList `json:"allowedIPs"`
	ClientAddressPool StringList `json:"clientAddressPool"`
	Endpoint          string     `json:"endpoint"`
	EndpointIp        string     `json:"endpointIp"`
	EndpointIP        string     `json:"endpointIP"`
	Uptime            string     `json:"uptime"`
	DownPkts          int64      `json:"downPkts"`
	DownBytes         int64      `json:"downBytes"`
	UpPkts            int64      `json:"upPkts"`
	UpBytes           int64      `json:"upBytes"`
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

// DetailLabels returns optional IP/network attributes for MQTT and Prometheus labels.
func (v *VpnStats) DetailLabels() VPNDetailLabels {
	return vpnDetailLabels(
		v.LocalIp,
		[]StringList{v.LocalNetwork, v.LocalNetworks, v.LocalSubnet, v.LocalSubnets},
		[]StringList{v.RemoteNetwork, v.RemoteNetworks, v.RemoteSubnet, v.RemoteSubnets},
		[]StringList{v.AllowedIps, v.AllowedIPs, v.ClientAddressPool},
		v.Endpoint,
		firstNonEmptyString(v.EndpointIp, v.EndpointIP),
	)
}
