package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
)

func (c *Client) GetAlert() (*model.Alert, error) {
	url := fmt.Sprintf("%s/%s/api/v2/sites/alert-count", c.Config.Host, c.OmadaCID)
	jsonStr := []byte(fmt.Sprintf(`{"siteIds":["%s"]}`, c.SiteId))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
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
	log.Info().Msg("Received data from alert endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from alert endpoint")

	alertsData := alertsResponse{}
	err = json.Unmarshal(body, &alertsData)
	if len(alertsData.Result) > 0 {
		firstAlert := alertsData.Result[0]
		return &firstAlert, err
	}
	return nil, err
}

type alertsResponse struct {
	Result []model.Alert `json:"result"`
}
