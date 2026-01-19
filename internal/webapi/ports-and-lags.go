package webapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/rs/zerolog/log"
)

func (c *Client) GetPortsAndLags(sw *model.Switch) error {
	url := fmt.Sprintf("%s/%s/api/v2/sites/%s/switches/%s", c.Config.Host, c.OmadaCID, c.SiteId, sw.Mac)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Info().Msg(fmt.Sprintf("Received data from ports endpoint for %s", sw.Mac))
	log.Debug().Bytes("data", body).Msg("Received data from ports endpoint")

	portdata := portResponse{}
	err = json.Unmarshal(body, &portdata)
	sw.Temp = portdata.Result.Temp
	sw.TotalPower = portdata.Result.TotalPower
	sw.Uplink = portdata.Result.Uplink
	sw.RxRate = portdata.Result.RxRate
	sw.TxRate = portdata.Result.TxRate
	sw.Ports = portdata.Result.Ports
	sw.Lags = portdata.Result.Lags
	return err
}
func (c *Client) GetApPorts(ap *model.AccessPoint) error {
	url := fmt.Sprintf("%s/%s/api/v2/sites/%s/eaps/%s/ports", c.Config.Host, c.OmadaCID, c.SiteId, ap.Mac)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Info().Msg(fmt.Sprintf("Received data from ports endpoint for AP %s", ap.Mac))
	log.Debug().Bytes("data", body).Msg("Received data from ports endpoint for AP")

	portdata := apPortResponse{}
	err = json.Unmarshal(body, &portdata)
	ap.Ports = portdata.Result
	return err
}

func (c *Client) GetGatewayPorts(gw *model.Gateway) error {
	url := fmt.Sprintf("%s/%s/api/v2/sites/%s/gateways/%s", c.Config.Host, c.OmadaCID, c.SiteId, gw.Mac)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Info().Msg(fmt.Sprintf("Received data from ports endpoint for %s", gw.Mac))
	log.Debug().Bytes("data", body).Msg("Received data from ports endpoint")

	portdata := gatewayResponse{}
	err = json.Unmarshal(body, &portdata)
	for i, p := range portdata.Result.Ports {
		portdata.Result.Ports[i].MaxSpeed = portdata.GetMaxLinkSpeed(p.Port)
	}
	gw.Temp = portdata.Result.Temp
	gw.Ports = portdata.Result.Ports
	gw.RxRate = portdata.Result.RxRate
	gw.TxRate = portdata.Result.TxRate
	return err
}

type portResponse struct {
	Result model.Switch `json:"result"`
}
type apPortResponse struct {
	Result []model.AccessPointPort `json:"result"`
}

type gatewayResponse struct {
	Result struct {
		Temp        float64             `json:"temp"`
		RxRate      float64             `json:"rxRate"`
		TxRate      float64             `json:"txRate"`
		PortConfigs []gatewayPortConfig `json:"portConfigs"`
		Ports       []model.GatewayPort `json:"portStats"`
	} `json:"result"`
}

type gatewayPortConfig struct {
	Port     int8      `json:"port"`
	PortCaps []PortCap `json:"portCap"`
}
type PortCap struct {
	LinkSpeed int8
}

func (ps *gatewayResponse) GetMaxLinkSpeed(port int8) int32 {
	maxSpeed := 0
	for _, conf := range ps.Result.PortConfigs {
		if conf.Port == port {
			for _, cap := range conf.PortCaps {
				capSpeed := 0
				switch cap.LinkSpeed {
				case 0:
					capSpeed = 0
				case 1:
					capSpeed = 10
				case 2:
					capSpeed = 100
				case 3:
					capSpeed = 1000
				case 4:
					capSpeed = 2500
				case 5:
					capSpeed = 10000
				case 6:
					capSpeed = 5000
				case 7:
					capSpeed = 25000
				case 8:
					capSpeed = 100000
				case 9:
					capSpeed = 40000
				}
				if capSpeed > maxSpeed {
					maxSpeed = capSpeed
				}
			}
		}
	}
	return int32(maxSpeed)
}
