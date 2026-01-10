package model

type Switch struct {
	Device
	Uplink Uplink `json:"uplink"`
	Ports  []Port `json:"ports"`
	Lags   []Lag  `json:"lags"`
	//Labels
	FanStatus  int8  `json:"fanStatus"`
	PoeSupport bool  `json:"poeSupport"`
	PortNumber int8  `json:"portNum"`
	TotalPower int32 `json:"totalPower"`
	//Fields
	PoeRemain float64 `json:"poeRemain"`
}

func (sw *Switch) GetPoeSupport() string {
	switch sw.PoeSupport {
	case true:
		return "Yes"
	case false:
		return "No"
	}
	return ""
}
