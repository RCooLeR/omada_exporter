package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
)

const controllerUpgradeChannelTimeout = 5 * time.Second

// GetController returns cached controller status data combined with upgrade-channel information.
func (c *Client) GetController() (*model.Controller, error) {
	return c.FetchCached("webapi:controller", c.getControllerFresh)
}

// getControllerFresh fetches controller status and available upgrade channels
// from separate Web API endpoints and merges them into one Controller value.
func (c *Client) getControllerFresh() (*model.Controller, error) {
	omadaCID, _ := c.ContextIDs()
	url := fmt.Sprintf("%s/%s/api/v2/settings/system/status", c.Config.Host, omadaCID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := api.ReadResponseBody(resp, "controllerStatus")
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Received data from controllerStatus endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from controllerStatus endpoint")

	if err := api.ValidateAPIResponse(body, "controllerStatus"); err != nil {
		return nil, err
	}

	controllerData := controllerResponse{}
	err = json.Unmarshal(body, &controllerData)
	if err != nil {
		return nil, err
	}

	upgradeList, err := c.getControllerUpgradeList()
	if err != nil {
		log.Warn().Err(err).Msg("failed to get controller upgrade channels")
	} else {
		controllerData.Result.UpgradeList = upgradeList
	}

	return &controllerData.Result, nil
}

func (c *Client) getControllerUpgradeList() ([]model.ControllerUpdate, error) {
	omadaCID, _ := c.ContextIDs()
	url := fmt.Sprintf("%s/%s/api/v2/maintenance/software/channelUpdate", c.Config.Host, omadaCID)
	ctx, cancel := context.WithTimeout(context.Background(), c.controllerUpgradeTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := api.ReadResponseBody(resp, "controllerChannelUpdate")
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Received data from controllerChannelUpdate endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from controllerChannelUpdate endpoint")

	if err := api.ValidateAPIResponse(body, "controllerChannelUpdate"); err != nil {
		return nil, err
	}

	controllerUpdateData := controllerUpdatesResponse{}
	err = json.Unmarshal(body, &controllerUpdateData)
	if err != nil {
		return nil, err
	}

	return controllerUpdateData.Result.UpgradeList, nil
}

func (c *Client) controllerUpgradeTimeout() time.Duration {
	timeout := controllerUpgradeChannelTimeout
	if c.Config == nil || c.Config.Timeout <= 0 {
		return timeout
	}

	configured := time.Duration(c.Config.Timeout) * time.Second
	if configured < timeout {
		return configured
	}
	return timeout
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
