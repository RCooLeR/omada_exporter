package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
)

func (c *Client) GetVpnStats() ([]model.VpnStats, error) {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return nil, fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/tunnel?page=1&pageSize=1000", c.Config.Host, c.OmadaCID, c.SiteId)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
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
	log.Info().Msg("Received data from VPNStats endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from VPNStats endpoint")

	vpnstatsdata := VpnStatsResponse{}
	err = json.Unmarshal(body, &vpnstatsdata)

	return vpnstatsdata.Result.Data, err
}

type VpnStatsResponse struct {
	Result struct {
		Data []model.VpnStats `json:"data"`
	} `json:"result"`
}
