package model

// Olt represents OLT data returned by Omada.
type Olt struct {
	Device
	Up       float64 `json:"up"`
	Down     float64 `json:"down"`
	OnuCount int     `json:"onuCount"`
}
