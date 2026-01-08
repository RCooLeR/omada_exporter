package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	log "github.com/rs/zerolog/log"
)

func (c *Client) GetVpn() ([]Vpn, error) {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return nil, fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/vpn", c.Config.Host, c.omadaCID, c.SiteId)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Info().Err(err).Msgf("Error creating GET request for %s", url)
		return nil, err
	}

	resp, err := c.makeOpenApiRequest(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Debug().Bytes("data", body).Msg("Received data from VPN endpoint")

	vpndata := vpnResponse{}
	err = json.Unmarshal(body, &vpndata)

	return vpndata.Result.Data, err
}

type vpnResponse struct {
	Result VpnData `json:"result"`
}
type VpnData struct {
	Data []Vpn `json:"data"`
}

type Vpn struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Purpose  int8   `json:"purpose"`
	VpnMode  int8   `json:"clientVpnType1"`
	VpnType  int8   `json:"clientVpnType2"`
	RemoteIp string `json:"remoteIp"`
	Status   bool   `json:"status"`
}
