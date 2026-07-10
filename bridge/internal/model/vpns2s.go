package model

import "strings"

// SiteToSiteVpnSummary represents summary data for a site-to-site VPN.
type SiteToSiteVpnSummary struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Status            bool       `json:"status"`
	VpnType           int8       `json:"vpnType"`
	SiteVpnType       int8       `json:"siteVpnType"`
	NetworkType       int8       `json:"networkType"`
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
}

// GetVpnType converts the site-to-site VPN type code to a readable protocol label.
func (v *SiteToSiteVpnSummary) GetVpnType() string {
	return vpnTypeString(v.VpnType)
}

// GetSiteVpnType converts the site VPN type code to a readable configuration label.
func (v *SiteToSiteVpnSummary) GetSiteVpnType() string {
	return siteToSiteVpnTypeString(v.SiteVpnType)
}

// DetailLabels returns optional IP/network attributes for MQTT and Prometheus labels.
func (v *SiteToSiteVpnSummary) DetailLabels(localIP string) VPNDetailLabels {
	return vpnDetailLabels(
		localIP,
		[]StringList{v.LocalNetwork, v.LocalNetworks, v.LocalSubnet, v.LocalSubnets},
		[]StringList{v.RemoteNetwork, v.RemoteNetworks, v.RemoteSubnet, v.RemoteSubnets},
		[]StringList{v.AllowedIps, v.AllowedIPs, v.ClientAddressPool},
		"",
		"",
	)
}

// SiteToSiteVpnStats represents runtime statistics for a site-to-site VPN.
type SiteToSiteVpnStats struct {
	ID                string     `json:"id"`
	VpnID             string     `json:"vpnId"`
	Spi               int64      `json:"spi"`
	Name              string     `json:"name"`
	Direction         string     `json:"direction"`
	LocalPeerIP       string     `json:"localPeerIp"`
	RemotePeerIP      string     `json:"remotePeerIp"`
	LocalIP           string     `json:"localIp"`
	RemoteIP          string     `json:"remoteIp"`
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
	LocalSA           string     `json:"localSa"`
	RemoteSA          string     `json:"remoteSa"`
	Protocol          string     `json:"protocol"`
	AHAuthentication  string     `json:"ahAuthentication"`
	ESPAuthentication string     `json:"espAuthentication"`
	ESPEncryption     string     `json:"espEncryption"`
	InterfaceName     string     `json:"interfaceName"`
	VpnMode           int8       `json:"serverType"`
	VpnType           int8       `json:"vpnType"`
	DownPkts          int64      `json:"downPkts"`
	DownBytes         int64      `json:"downBytes"`
	UpPkts            int64      `json:"upPkts"`
	UpBytes           int64      `json:"upBytes"`
	Uptime            string     `json:"uptime"`
	Port              int32      `json:"port"`
	ConnectedNum      *int64     `json:"connectedNum"`
	DisconnectedNum   *int64     `json:"disconnectedNum"`
	TotalRemoteNum    int64      `json:"totalRemoteNum"`
	Status            int8       `json:"status"`
}

// GetVpnType converts the runtime site-to-site VPN type code to a readable protocol label.
func (v *SiteToSiteVpnStats) GetVpnType() string {
	return vpnTypeString(v.VpnType)
}

// GetVpnMode converts the runtime site-to-site VPN mode code to a readable role label.
func (v *SiteToSiteVpnStats) GetVpnMode() string {
	return vpnModeString(v.VpnMode)
}

// DetailLabels returns optional IP/network attributes for MQTT and Prometheus labels.
func (v *SiteToSiteVpnStats) DetailLabels(summary SiteToSiteVpnSummary) VPNDetailLabels {
	detail := vpnDetailLabels(
		v.LocalIP,
		[]StringList{v.LocalNetwork, v.LocalNetworks, v.LocalSubnet, v.LocalSubnets},
		[]StringList{v.RemoteNetwork, v.RemoteNetworks, v.RemoteSubnet, v.RemoteSubnets},
		[]StringList{v.AllowedIps, v.AllowedIPs, v.ClientAddressPool},
		v.Endpoint,
		firstNonEmptyString(v.EndpointIp, v.EndpointIP),
	)
	if detail.LocalNetworks == "" || detail.RemoteNetworks == "" || detail.AllowedIPs == "" {
		summaryDetail := summary.DetailLabels(v.LocalIP)
		if detail.LocalNetworks == "" {
			detail.LocalNetworks = summaryDetail.LocalNetworks
		}
		if detail.RemoteNetworks == "" {
			detail.RemoteNetworks = summaryDetail.RemoteNetworks
		}
		if detail.AllowedIPs == "" {
			detail.AllowedIPs = summaryDetail.AllowedIPs
		}
	}
	return detail
}

// SiteToSiteVpnPeerStats represents runtime statistics for a site-to-site VPN peer.
type SiteToSiteVpnPeerStats struct {
	ID         string     `json:"id"`
	VpnID      string     `json:"vpnId"`
	Name       string     `json:"name"`
	RemoteIP   string     `json:"remoteIp"`
	LocalIP    string     `json:"localIp"`
	AllowedIps StringList `json:"allowedIps"`
	AllowedIPs StringList `json:"allowedIPs"`
	Endpoint   string     `json:"endpoint"`
	EndpointIp string     `json:"endpointIp"`
	EndpointIP string     `json:"endpointIP"`
	DownPkts   *int64     `json:"downPkts"`
	DownBytes  int64      `json:"downBytes"`
	UpPkts     *int64     `json:"upPkts"`
	UpBytes    int64      `json:"upBytes"`
	LoginTime  int64      `json:"loginTime"`
	Port       int32      `json:"port"`
	Status     *int8      `json:"status"`
}

// GetStatus converts the peer status flag into a Prometheus-friendly numeric value.
func (v *SiteToSiteVpnPeerStats) GetStatus() (float64, bool) {
	if v.Status == nil {
		return 0, false
	}
	if *v.Status == 1 {
		return 1, true
	}
	return 0, true
}

// HasPacketStats reports whether packet counters were included in the response.
func (v *SiteToSiteVpnPeerStats) HasPacketStats() bool {
	return v.DownPkts != nil || v.UpPkts != nil
}

// DetailLabels returns optional IP/network attributes for MQTT and Prometheus labels.
func (v *SiteToSiteVpnPeerStats) DetailLabels(summary SiteToSiteVpnSummary) VPNDetailLabels {
	detail := vpnDetailLabels(
		v.LocalIP,
		nil,
		nil,
		[]StringList{v.AllowedIps, v.AllowedIPs},
		v.Endpoint,
		firstNonEmptyString(v.EndpointIp, v.EndpointIP),
	)
	summaryDetail := summary.DetailLabels(v.LocalIP)
	detail.LocalNetworks = summaryDetail.LocalNetworks
	detail.RemoteNetworks = summaryDetail.RemoteNetworks
	if detail.AllowedIPs == "" {
		detail.AllowedIPs = summaryDetail.AllowedIPs
	}
	return detail
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
