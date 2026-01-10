package model

type PortStatus struct {
	LinkStatus int8    `json:"linkStatus"`
	LinkSpeed  int8    `json:"linkSpeed"`
	Poe        bool    `json:"poe"`
	PoePower   float64 `json:"poePower"`
	Rx         float64 `json:"rx"`
	Tx         float64 `json:"tx"`
	RxRate     float64 `json:"rxRate"`
	TxRate     float64 `json:"txRate"`
}

func (ps *PortStatus) GetLinkStatus() string {
	switch ps.LinkStatus {
	case 0:
		return "Disconnected"
	case 1:
		return "Connected"
	default:
		return "Unknown"
	}
}

func (ps *PortStatus) GetLinkSpeed() int32 {
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
