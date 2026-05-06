package model

// Alert represents alert data returned by Omada.
type Alert struct {
	AlertNum int  `json:"alertNum"`
	Obscured bool `json:"obscured"`
}
