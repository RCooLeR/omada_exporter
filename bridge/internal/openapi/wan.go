package openapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/rs/zerolog/log"
)

// GetWans fetches WAN status data for the provided gateway from the Open API,
// decodes the response, and stores the resulting WAN list on gw.Wans.
func (c *Client) GetWans(gw *model.Gateway) error {
	if err := c.requireOpenAPICredentials(); err != nil {
		return err
	}
	omadaCID, siteID := c.ContextIDs()
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/wan-status", c.Config.Host, omadaCID, siteID, gw.Mac)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.MakeOpenApiRequest(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := api.ReadResponseBody(resp, "WAN status")
	if err != nil {
		return err
	}
	log.Info().Msg(fmt.Sprintf("Received data from WAN endpoint for %s", gw.Mac))
	log.Debug().Bytes("data", body).Msg("Received data from WAN endpoint")
	if err := api.ValidateAPIResponse(body, "WAN status"); err != nil {
		return err
	}

	wandata := wanResponse{}
	if err := json.Unmarshal(body, &wandata); err != nil {
		return err
	}
	gw.Wans = wandata.Result
	return nil
}

// wanResponse wraps the Open API payload returned by the gateway WAN status endpoint.
type wanResponse struct {
	Result []model.Wan `json:"result"`
}
