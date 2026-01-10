package model

type Vpn struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Purpose  int8   `json:"purpose"`
	VpnMode  int8   `json:"clientVpnType1"`
	VpnType  int8   `json:"clientVpnType2"`
	RemoteIp string `json:"remoteIp"`
	Status   bool   `json:"status"`
}

func (v *Vpn) GetPurpose() string {
	switch v.Purpose {
	case 0:
		return "Site-to-Site"
	case 1:
		return "Client-to-Site"
	}
	return ""
}
func (v *Vpn) GetVpnMode() string {
	switch v.VpnMode {
	case 0:
		return "Server"
	case 1:
		return "Client"
	}
	return ""
}
func (v *Vpn) GetVpnType() string {
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
