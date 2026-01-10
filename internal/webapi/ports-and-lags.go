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
	sw.TotalPower = portdata.Result.TotalPower
	sw.Uplink = portdata.Result.Uplink
	sw.Ports = portdata.Result.Ports
	sw.Lags = portdata.Result.Lags
	return err
}

type portResponse struct {
	Result model.Switch `json:"result"`
}
