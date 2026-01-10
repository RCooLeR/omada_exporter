package model

import (
	"strconv"
	"strings"
)

type Lag struct {
	LagId     int8      `json:"lagId"`
	LagType   int8      `json:"lagType"`
	Name      string    `json:"name"`
	Ports     []int8    `json:"ports"`
	LagStatus LagStatus `json:"lagStatus"`
}

func (l *Lag) GetLagType() string {
	switch l.LagType {
	case 1:
		return "Static"
	case 2:
		return "LACP"
	case 3:
		return "LACP Active"
	case 4:
		return "LACP Passive"
	}
	return "Unknown"
}

func (l *Lag) GetPorts() string {
	strs := make([]string, len(l.Ports))
	for i, p := range l.Ports {
		strs[i] = strconv.Itoa(int(p)) // преобразуем int8 → int → string
	}
	return strings.Join(strs, ",")
}
