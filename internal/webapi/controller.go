package webapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
)

// GetController returns cached controller status data combined with upgrade-channel information.
func (c *Client) GetController() (*model.Controller, error) {
	return api.FetchCached(c.Client, "webapi:controller", c.getControllerFresh)
}

// getControllerFresh fetches controller status and available upgrade channels
// from separate Web API endpoints and merges them into one Controller value.
func (c *Client) getControllerFresh() (*model.Controller, error) {
	url := fmt.Sprintf("%s/%s/api/v2/settings/system/status", c.Config.Host, c.OmadaCID)
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
	log.Info().Msg("Received data from controllerStatus endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from controllerStatus endpoint")

	controllerData := controllerResponse{}
	err = json.Unmarshal(body, &controllerData)

	url = fmt.Sprintf("%s/%s/api/v2/maintenance/software/channelUpdate", c.Config.Host, c.OmadaCID)
	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err = c.MakeLoggedInRequest(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Received data from controllerStatus endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from controllerStatus endpoint")

	controllerUpdateData := controllerUpdatesResponse{}
	err = json.Unmarshal(body, &controllerUpdateData)
	controllerData.Result.UpgradeList = controllerUpdateData.Result.UpgradeList

	return &controllerData.Result, err
}

// controllerResponse represents the Web API response for controller data.
type controllerResponse struct {
	Result model.Controller `json:"result"`
}

// controllerUpdatesResponse represents the Web API response for controller updates.
type controllerUpdatesResponse struct {
	Result struct {
		UpgradeList []model.ControllerUpdate `json:"upgradeList"`
	} `json:"result"`
}
