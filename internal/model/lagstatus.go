package model

type LagStatus struct {
	LinkStatus int8    `json:"linkStatus"`
	LinkSpeed  int8    `json:"linkSpeed"`
	Rx         float64 `json:"rx"`
	Tx         float64 `json:"tx"`
	RxRate     float64 `json:"rxRate"`
	TxRate     float64 `json:"txRate"`
	Ports      []int8  `json:"ports"`
}

func (ls *LagStatus) GetLinkStatus() string {
	switch ls.LinkStatus {
	case 0:
		return "Disconnected"
	case 1:
		return "Connected"
	default:
		return "Unknown"
	}
}

func (ls *LagStatus) GetLinkSpeed() int32 {
	switch ls.LinkSpeed {
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
func (ls *LagStatus) GetTotalLagSpeed(sw *Switch) int32 {
	var speed int32
	speed = 0
	for _, port := range ls.Ports {
		for _, swPort := range sw.Ports {
			if port == swPort.Port {
				speed += swPort.PortStatus.GetLinkSpeed()
			}
		}
	}
	return speed
}
