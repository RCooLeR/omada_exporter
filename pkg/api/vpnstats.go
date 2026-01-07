package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	log "github.com/rs/zerolog/log"
)

func (c *Client) GetVpnStats() ([]VpnStats, error) {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return nil, fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/tunnel?page=1&pageSize=1000", c.Config.Host, c.omadaCID, c.SiteId)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
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
	log.Debug().Bytes("data", body).Msg("Received data from VPNStats endpoint")

	vpnstatsdata := VpnStatsResponse{}
	err = json.Unmarshal(body, &vpnstatsdata)

	return vpnstatsdata.Result.Data, err
}

type VpnStatsResponse struct {
	Result VpnStatsData `json:"result"`
}
type VpnStatsData struct {
	Data []VpnStats `json:"data"`
}

type VpnStats struct {
	Name          string `json:"vpnName"`
	InterfaceName string `json:"interfaceName"`
	VpnMode       int8   `json:"serverType"`
	VpnType       int8   `json:"vpnType"`
	LocalIp       string `json:"localIp"`
	RemoteIp      string `json:"remoteIp"`
	Uptime        string `json:"uptime"`
	DownBytes     int64  `json:"downBytes"`
	UpBytes       int64  `json:"upBytes"`
}
