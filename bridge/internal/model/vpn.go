package model

// Vpn represents VPN summary information returned by the Omada Open API.
type Vpn struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Purpose  int8   `json:"purpose"`
	VpnMode  int8   `json:"clientVpnType1"`
	VpnType  int8   `json:"clientVpnType2"`
	RemoteIp string `json:"remoteIp"`
	Status   bool   `json:"status"`
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
