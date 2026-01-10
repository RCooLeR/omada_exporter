package model

import (
	"strings"
)

type NetworkClient struct {
	Mac            string `json:"mac"`
	Ip             string `json:"ip"`
	VlanId         int8   `json:"vid"`
	ConnectType    int8   `json:"connect_type"`
	Name           string `json:"name"`
	SystemName     string `json:"systemName"`
	HostName       string `json:"hostName"`
	DeviceType     string `json:"deviceType"`
	DeviceCategory string `json:"deviceCategory"`
	Vendor         string `json:"vendor"`

	ConnectDevType string `json:"connectDevType"`

	GatewayMac  string `json:"gatewayMac"`
	GatewayName string `json:"gatewayName"`
	SwitchMac   string `json:"switchMac"`
	SwitchName  string `json:"switchName"`
	Port        int8   `json:"port"`
	LagId       int8   `json:"lagId"`

	Wireless bool   `json:"wireless"`
	ApMac    string `json:"apMac"`
	ApName   string `json:"apName"`
	WifiMode int8   `json:"wifiMode"`
	Ssid     string `json:"ssid"`

	Activity       float64 `json:"activity"`
	UploadActivity float64 `json:"uploadActivity"`
	TrafficDown    float64 `json:"trafficDown"`
	TrafficUp      float64 `json:"trafficUp"`

	Rssi        float64 `json:"rssi"`
	SignalLevel float64 `json:"signalLevel"`
	SignalNoise float64 `json:"snr"`
	RxRate      float64 `json:"rxRate"`
	TxRate      float64 `json:"txRate"`
}

func (s *NetworkClient) GetName() string {
	return strings.TrimSpace(s.Name)
}
func (c *NetworkClient) GetWifiMode() string {
	mapping := map[int8]string{
		0: "802.11a",
		1: "802.11b",
		2: "802.11g",
		3: "802.11na",
		4: "802.11ng",
		5: "802.11ac",
		6: "802.11axa",
		7: "802.11axg",
		8: "802.11beg",
		9: "802.11bea",
	}
	formatted, ok := mapping[c.WifiMode]
	if !ok {
		return ""
	}
	return formatted
}

func (c *NetworkClient) GetConnectType() string {
	switch c.ConnectType {
	case 0:
		return "wireless guest"
	case 1:
		return "wireless user"
	case 2:
		return "wired user"
	}
	return ""
}
