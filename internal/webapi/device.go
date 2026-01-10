package webapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/openapi"
	"github.com/rs/zerolog/log"
)

func (c *Client) GetDevices() ([]model.DevicesInterface, error) {
	//hack for keeping logic in separate dirs
	openClient := &openapi.Client{
		Client: c.Client,
	}
	url := fmt.Sprintf("%s/%s/api/v2/sites/%s/devices", c.Config.Host, c.OmadaCID, c.SiteId)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Received data from devices endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from devices endpoint")

	var devicedata devicesResponse
	err = json.Unmarshal(body, &devicedata)

	for _, d := range devicedata.Result {
		switch dev := d.(type) {
		case *model.Switch:
			err := c.GetPortsAndLags(dev)
			if err != nil {
				log.Error().Err(err).Msg("Error getting ports and lags")
			}
		case *model.Gateway:
			err := openClient.GetWans(dev)
			if err != nil {
				log.Error().Err(err).Msg("Error getting wans")
			}

		}
	}

	return devicedata.Result, nil
}

type devicesResponse struct {
	Result []model.DevicesInterface
}

func (d *devicesResponse) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Result []json.RawMessage `json:"result"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	for _, raw := range tmp.Result {
		var hint struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal(raw, &hint); err != nil {
			return err
		}

		switch hint.Type {
		case "gateway":
			var gw model.Gateway
			if err := json.Unmarshal(raw, &gw); err != nil {
				return err
			}
			d.Result = append(d.Result, &gw)
		case "switch":
			var sw model.Switch
			if err := json.Unmarshal(raw, &sw); err != nil {
				return err
			}
			d.Result = append(d.Result, &sw)
		case "ap":
			var ap model.AccessPoint
			if err := json.Unmarshal(raw, &ap); err != nil {
				return err
			}
			d.Result = append(d.Result, &ap)
		case "olt":
			var olt model.Olt
			if err := json.Unmarshal(raw, &olt); err != nil {
				return err
			}
			d.Result = append(d.Result, &olt)
		default:
			return fmt.Errorf("unknown device type: %s", hint.Type)
		}
	}

	return nil
}
