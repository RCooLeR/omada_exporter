package model

import (
	"encoding/json"
	"strings"
)

// NetworkClient represents a client device connected to the Omada network.
type NetworkClient struct {
	Mac            string `json:"mac"`
	Ip             string `json:"ip"`
	VlanId         int32  `json:"vid"`
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

// UnmarshalJSON accepts both older snake_case OpenAPI client fields and the
// camelCase fields returned by Fusion's v1 clients endpoint.
func (c *NetworkClient) UnmarshalJSON(data []byte) error {
	type Alias NetworkClient
	var raw struct {
		Alias
		ConnectTypeSnake int8    `json:"connect_type"`
		ConnectTypeCamel *int8   `json:"connectType"`
		GatewayMacSnake  string  `json:"gateway_mac"`
		GatewayNameSnake string  `json:"gateway_name"`
		SwitchMacSnake   string  `json:"switch_mac"`
		SwitchNameSnake  string  `json:"switch_name"`
		LagIdSnake       int8    `json:"lag_id"`
		WifiModeSnake    int8    `json:"wifi_mode"`
		WifiModeCamel    *int8   `json:"wifiMode"`
		SignalLevelCamel float64 `json:"signalLevel"`
		SignalNoiseCamel float64 `json:"signalNoise"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*c = NetworkClient(raw.Alias)
	if raw.ConnectTypeCamel != nil {
		c.ConnectType = *raw.ConnectTypeCamel
	} else {
		c.ConnectType = raw.ConnectTypeSnake
	}
	if raw.WifiModeCamel != nil {
		c.WifiMode = *raw.WifiModeCamel
	} else {
		c.WifiMode = raw.WifiModeSnake
	}
	if c.GatewayMac == "" {
		c.GatewayMac = raw.GatewayMacSnake
	}
	if c.GatewayName == "" {
		c.GatewayName = raw.GatewayNameSnake
	}
	if c.SwitchMac == "" {
		c.SwitchMac = raw.SwitchMacSnake
	}
	if c.SwitchName == "" {
		c.SwitchName = raw.SwitchNameSnake
	}
	if c.LagId == 0 {
		c.LagId = raw.LagIdSnake
	}
	if c.SignalLevel == 0 {
		c.SignalLevel = raw.SignalLevelCamel
	}
	if c.SignalNoise == 0 {
		c.SignalNoise = raw.SignalNoiseCamel
	}
	return nil
}

// GetName returns the trimmed client name reported by Omada.
func (s *NetworkClient) GetName() string {
	return strings.TrimSpace(s.Name)
}

// GetWifiMode maps the Wi-Fi mode code to a readable 802.11 standard label.
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

// GetConnectType maps the connection type code to a wired or wireless client label.
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
