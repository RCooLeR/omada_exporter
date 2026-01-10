package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
)

func (c *Client) GetVpn() ([]model.Vpn, error) {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return nil, fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/vpn", c.Config.Host, c.OmadaCID, c.SiteId)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error().Err(err).Msgf("Error creating GET request for %s", url)
		return nil, err
	}
	resp, err := c.MakeOpenApiRequest(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Received data from VPN endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from VPN endpoint")

	vpndata := vpnResponse{}
	err = json.Unmarshal(body, &vpndata)

	return vpndata.Result.Data, err
}

type vpnResponse struct {
	Result struct {
		Data []model.Vpn `json:"data"`
	} `json:"result"`
}
