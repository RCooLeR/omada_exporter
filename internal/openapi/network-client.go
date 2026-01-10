package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/rs/zerolog/log"
)

func (c *Client) GetNetworkClients() ([]model.NetworkClient, error) {
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/clients", c.Config.Host, c.OmadaCID, c.SiteId)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("page", "1")
	q.Add("pageSize", "1000")
	q.Add("filters.active", "true")

	req.URL.RawQuery = q.Encode()

	resp, err := c.MakeOpenApiRequest(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Received data from clients endpoint")
	log.Debug().Bytes("data", body).Msg("Received data from clients endpoint")

	clientdata := clientResponse{}
	err = json.Unmarshal(body, &clientdata)

	return clientdata.Result.Data, err
}

type clientResponse struct {
	Result struct {
		Data []model.NetworkClient `json:"data"`
	} `json:"result"`
}
