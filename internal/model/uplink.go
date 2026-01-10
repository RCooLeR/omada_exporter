package model

type Uplink struct {
	LinkSpeed int8   `json:"linkSpeed"`
	IP        string `json:"ip"`
	Mac       string `json:"mac"`
	Port      int8   `json:"port"`
	Type      string `json:"type"`
}

func (up *Uplink) GetLinkSpeed() int32 {
	switch up.LinkSpeed {
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
	default:
		return 0
	}
}
