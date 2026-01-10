package model

type Wan struct {
	//Labels
	Port          int8   `json:"port"`
	Name          string `json:"name"`
	Desc          string `json:"portDesc"`
	Type          int8   `json:"type"`
	Ip            string `json:"ip"`
	Proto         string `json:"proto"`
	Status        int8   `json:"status"`
	InternetState int8   `json:"internetState"`
	LinkSpeed     int8   `json:"speed"`
	//Fields
	RxRate  float64 `json:"rxRate"`
	TxRate  float64 `json:"txRate"`
	Latency int8    `json:"latency"`
}

func (w *Wan) GetLinkSpeed() int32 {
	if 0 == w.Status {
		return 0
	}
	switch w.LinkSpeed {
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
func (wan *Wan) GetStatus() string {
	switch wan.Status {
	case 0:
		return "Enabled"
	case 1:
		return "Disabled"
	default:
		return "Unknown"
	}
}
func (wan *Wan) GetType() string {
	switch wan.Type {
	case 0:
		return "WAN"
	case 1:
		return "WAN/LAN"
	case 2:
		return "LAN"
	}
	return "WAN"
}
