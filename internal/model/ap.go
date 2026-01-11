package model

type AccessPoint struct {
	Device
	Uplink Uplink `json:"wiredUplink"`
	//Labels
	AnyPoeEnable   bool   `json:"anyPoeEnable"`
	WirelessLinked bool   `json:"wirelessLinked"`
	WlanGroup      string `json:"wlanGroup"`
	//Fields
	Wp2GHz   *Radio `json:"wp2g,omitempty"`
	Wp5GHz   *Radio `json:"wp5g,omitempty"`
	Wp5GHz_1 *Radio `json:"wp5g1,omitempty"`
	Wp5GHz_2 *Radio `json:"wp5g2,omitempty"`
	Wp6GHz   *Radio `json:"wp6g,omitempty"`
}
