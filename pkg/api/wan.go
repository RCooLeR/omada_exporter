package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	log "github.com/rs/zerolog/log"
)

func (c *Client) GetWans(gatewayMac string) ([]Wan, error) {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return nil, fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/wan-status", c.Config.Host, c.omadaCID, c.SiteId, gatewayMac)
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
	log.Debug().Bytes("data", body).Msg("Received data from WAN endpoint")

	wandata := wanResponse{}
	err = json.Unmarshal(body, &wandata)

	return wandata.Result, err
}

type wanResponse struct {
	Result []Wan `json:"result"`
}

type Wan struct {
	Port          float64 `json:"port"`
	Name          string  `json:"name"`
	Desc          string  `json:"portDesc"`
	Type          int8    `json:"type"`
	Ip            string  `json:"ip"`
	Proto         string  `json:"proto"`
	Status        int8    `json:"status"`
	InternetState int8    `json:"internetState"`
	LinkSpeed     float64 `json:"speed"`
	RxRate        float64 `json:"rxRate"`
	TxRate        float64 `json:"txRate"`
	Latency       int8    `json:"latency"`
}
