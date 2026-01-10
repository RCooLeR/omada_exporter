package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/rs/zerolog/log"
)

func (c *Client) GetWans(gw *model.Gateway) error {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/wan-status", c.Config.Host, c.OmadaCID, c.SiteId, gw.Mac)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.MakeOpenApiRequest(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Info().Msg(fmt.Sprintf("Received data from WAN endpoint for %s", gw.Mac))
	log.Debug().Bytes("data", body).Msg("Received data from WAN endpoint")

	wandata := wanResponse{}
	err = json.Unmarshal(body, &wandata)
	gw.Wans = wandata.Result
	return err
}

type wanResponse struct {
	Result []model.Wan `json:"result"`
}
