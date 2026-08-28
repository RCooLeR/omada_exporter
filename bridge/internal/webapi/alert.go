package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
)

// GetAlert returns cached site alert counts loaded from the Web API.
func (c *Client) GetAlert() (*model.Alert, error) {
	return c.FetchCached("webapi:alert", c.getAlertFresh)
}

// getAlertFresh posts the site alert-count request and returns the first alert
// summary from the Web API response when one is present.
func (c *Client) getAlertFresh() (*model.Alert, error) {
	omadaCID, siteID := c.ContextIDs()
	url := fmt.Sprintf("%s/%s/api/v2/sites/alert-count", c.Config.Host, omadaCID)
	requestBody, err := json.Marshal(struct {
		SiteIDs []string `json:"siteIds"`
	}{SiteIDs: []string{siteID}})
	if err != nil {
		return nil, fmt.Errorf("encode alert request: %w", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := api.ReadResponseBody(resp, "alert")
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Received data from alert endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from alert endpoint")

	if err := api.ValidateAPIResponse(body, "alert"); err != nil {
		return nil, err
	}

	alertsData := alertsResponse{}
	err = json.Unmarshal(body, &alertsData)
	if err != nil {
		return nil, err
	}
	if len(alertsData.Result) > 0 {
		firstAlert := alertsData.Result[0]
		return &firstAlert, nil
	}
	return nil, fmt.Errorf("alert response did not include a result for site %s", siteID)
}

// alertsResponse represents the Web API response for alerts.
type alertsResponse struct {
	Result []model.Alert `json:"result"`
}
