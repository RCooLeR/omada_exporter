package model

// Vpn represents VPN summary information returned by the Omada Open API.
type Vpn struct {
	Id                string     `json:"id"`
	Name              string     `json:"name"`
	Purpose           int8       `json:"purpose"`
	VpnMode           int8       `json:"clientVpnType1"`
	VpnType           int8       `json:"clientVpnType2"`
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
	Status            bool       `json:"status"`
}

// GetPurpose maps the VPN purpose code to a site-to-site or client-to-site label.
func (v *Vpn) GetPurpose() string {
	switch v.Purpose {
	case 0:
		return "Site-to-Site"
	case 1:
		return "Client-to-Site"
	}
	return ""
}

// GetVpnMode converts the VPN mode code to a readable role label.
func (v *Vpn) GetVpnMode() string {
	return vpnModeString(v.VpnMode)
}

// GetVpnType converts the VPN type code to a readable protocol label.
func (v *Vpn) GetVpnType() string {
	return vpnTypeString(v.VpnType)
}

// DetailLabels returns optional IP/network attributes for MQTT and Prometheus labels.
func (v *Vpn) DetailLabels() VPNDetailLabels {
	return vpnDetailLabels(
		v.LocalIp,
		[]StringList{v.LocalNetwork, v.LocalNetworks, v.LocalSubnet, v.LocalSubnets},
		[]StringList{v.RemoteNetwork, v.RemoteNetworks, v.RemoteSubnet, v.RemoteSubnets},
		[]StringList{v.AllowedIps, v.AllowedIPs, v.ClientAddressPool},
		v.Endpoint,
		firstNonEmptyString(v.EndpointIp, v.EndpointIP),
	)
}
