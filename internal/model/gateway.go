package model

type Gateway struct {
	Device
	Wans []Wan
	Isps []Isp
}
