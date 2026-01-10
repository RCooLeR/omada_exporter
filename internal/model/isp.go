package model

import "fmt"

type Isp struct {
	GatewayName   string `json:"gateway_name"`
	GatewayMac    string `json:"gateway_mac"`
	GatewayStatus int8   `json:"gateway_status"`
	//Labels
	Name             string `json:"name"`
	Port             int8   `json:"port"`
	Status           int8   `json:"status"`
	IP               string `json:"ip"`
	LoadBalance      string `json:"loadBalance"`
	MaxBandwidth     int32  `json:"maxBandwidth"`
	DownloadSpeedSet int32  `json:"downloadSpeedSet"`
	//Fields
	DownloadSpeed float64 `json:"downloadSpeed,string"`
	UploadSpeed   float64 `json:"uploadSpeed,string"`
}

func (isp *Isp) GetStatus() string {
	switch isp.Status {
	case 1:
		return "Online"
	case 0:
		return "Offline"
	default:
		return "Unknown"
	}
}

func (isp *Isp) GetGatewayStatus() string {
	switch isp.Status {
	case 1:
		return "Online"
	case 0:
		return "Offline"
	default:
		return "Unknown"
	}
}
func (isp *Isp) GetMaxBandwidth() string {
	b := isp.MaxBandwidth

	switch {
	case b >= 1000_000:
		return fmt.Sprintf("%.0f Tbps", float64(b)/1000_000)
	case b >= 1000:
		return fmt.Sprintf("%.1f Gbps", float64(b)/1000)
	default:
		return fmt.Sprintf("%d Mbps", b)
	}
}
