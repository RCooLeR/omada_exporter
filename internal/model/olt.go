package model

type Olt struct {
	Device
	Up       float64 `json:"up"`
	Down     float64 `json:"down"`
	OnuCount int     `json:"onuCount"`
}
