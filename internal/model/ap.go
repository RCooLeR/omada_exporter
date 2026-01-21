package model

import "fmt"

type AccessPoint struct {
	Device
	Uplink Uplink `json:"wiredUplink"`
	//Labels
	AnyPoeEnable   bool              `json:"anyPoeEnable"`
	WirelessLinked bool              `json:"wirelessLinked"`
	WlanGroup      string            `json:"wlanGroup"`
	Ports          []AccessPointPort `json:"lanPortSettings"`
	DeviceMisc     struct {
		LanPortsNum uint8 `json:"lanPortsNum"`
	} `json:"deviceMisc"`
	//Fields
	Wp2GHz   *Radio `json:"wp2g,omitempty"`
	Wp5GHz   *Radio `json:"wp5g,omitempty"`
	Wp5GHz_2 *Radio `json:"wp5g2,omitempty"`
	Wp6GHz   *Radio `json:"wp6g,omitempty"`
}

type AccessPointPort struct {
	Id           string  `json:"id"`
	Name         string  `json:"name"`
	IsUplinkPort bool    `json:"uplinkPort"`
	LinkStatus   int8    `json:"linkStatus"`
	LinkSpeed    int8    `json:"speed"`
	Poe          bool    `json:"supportPoe"`
	PoeEnabled   bool    `json:"poeOutEnable"`
	PoePower     float64 `json:"poePower"`
}

func (ps *AccessPointPort) GetLinkStatus() string {
	switch ps.LinkStatus {
	case 0:
		return "Disconnected"
	case 1:
		return "Connected"
	default:
		return "Unknown"
	}
}

func (ps *AccessPointPort) GetLinkSpeed() int32 {
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

func (ps *AccessPointPort) GetLinkSpeedLabel() string {
	label := ""
	if ps.Poe && ps.PoeEnabled {
		if 0 < ps.PoePower {
			label += "⚡ " + fmt.Sprintf("%.0f", ps.PoePower) + "w"
		} else {
			label += "⚡ "
		}
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
