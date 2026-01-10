package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
)

func (c *Client) GetIsp() ([]model.Isp, error) {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return nil, fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/dashboard/gateway/isp/load", c.Config.Host, c.OmadaCID, c.SiteId)
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
	log.Info().Msg("Received data from ISP endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from ISP endpoint")

	ispdata := ispResponse{}
	err = json.Unmarshal(body, &ispdata)
	var result []model.Isp
	for _, d := range ispdata.Result.Data {
		for _, isp := range d.IspInfo.IspArr {
			isp.GatewayName = d.GatewayName
			isp.GatewayMac = d.GatewayMac
			isp.GatewayStatus = d.GatewayStatus
			result = append(result, isp)
		}
	}

	return result, err
}

type ispResponse struct {
	Result struct {
		Data []struct {
			GatewayName   string `json:"name"`
			GatewayMac    string `json:"mac"`
			GatewayStatus int8   `json:"status"`
			IspInfo       struct {
				IspArr []model.Isp `json:"ispArr"`
			} `json:"IspInfo"`
		} `json:"data"`
	} `json:"result"`
}
