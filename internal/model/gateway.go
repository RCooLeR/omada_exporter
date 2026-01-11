package model

import "fmt"

type Gateway struct {
	Device
	Wans  []Wan
	Isps  []Isp
	Ports []GatewayPort
}
type GatewayPort struct {
	Port       int8 `json:"port"`
	MaxSpeed   int32
	Name       string  `json:"name"`
	Type       int8    `json:"type"`
	Operation  string  `json:"operation"`
	LinkStatus int8    `json:"status"`
	LinkSpeed  int8    `json:"speed"`
	Poe        bool    `json:"poe"`
	PoePower   float64 `json:"poePower"`
	Rx         float64 `json:"rx"`
	Tx         float64 `json:"tx"`
	RxRate     float64 `json:"rxRate"`
	TxRate     float64 `json:"txRate"`
}

func (p *GatewayPort) GetType() string {
	switch p.Type {
	case 1:
		return "Copper"
	case 2:
		return "Combo"
	case 3:
		return "SFP"
	default:
		return "Unknown"
	}
}

func (ps *GatewayPort) GetLinkStatus() string {
	switch ps.LinkStatus {
	case 0:
		return "Disconnected"
	case 1:
		return "Connected"
	default:
		return "Unknown"
	}
}

func (ps *GatewayPort) GetLinkSpeed() int32 {
	if 0 == ps.LinkStatus {
		return 0
	}
	switch ps.LinkSpeed {
	case 0:
		return 0
	case 1:
		return 10
	case 2:
		return 100
	case 3:
		return 1000
	case 4:
		return 2500
	case 5:
		return 10000
	case 6:
		return 5000
	case 7:
		return 25000
	case 8:
		return 100000
	case 9:
		return 40000
	default:
		return 0
	}
}

func (ps *GatewayPort) GetLinkSpeedLabel() string {
	label := ""
	if ps.Poe {
		label += "⚡ " + fmt.Sprintf("%.0f", ps.PoePower) + "w"
	}
	if "" != label {
		label += "  "
	}
	if ps.LinkStatus == 0 {
		return label + "⇅ -"
	}
	speedMap := map[int8]string{
		0: "⇅ -",
		1: "⇅ 10 Mbps",
		2: "⇅ 100 Mbps",
		3: "⇅ 1 Gbps",
		4: "⇅ 2.5 Gbps",
		5: "⇅ 10 Gbps",
		6: "⇅ 5 Gbps",
		7: "⇅ 25 Gbps",
		8: "⇅ 100 Gbps",
		9: "⇅ 40 Gbps",
	}
	speedLabel, ok := speedMap[ps.LinkSpeed]
	if !ok {
		speedLabel = "⇅ %"
	}
	return label + speedLabel
}
