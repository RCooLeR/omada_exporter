package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/rs/zerolog/log"
)

// GetNetworkClients returns cached network client inventory loaded from the Open API.
func (c *Client) GetNetworkClients() ([]model.NetworkClient, error) {
	return api.FetchCached(c.Client, "openapi:clients", c.getNetworkClientsFresh)
}

// getNetworkClientsFresh posts the active-client filter request to the Open API
// and returns the decoded client list for the current site.
func (c *Client) getNetworkClientsFresh() ([]model.NetworkClient, error) {
	url := fmt.Sprintf("%s/openapi/v2/%s/sites/%s/clients", c.Config.Host, c.OmadaCID, c.SiteId)
	requestBody, err := json.Marshal(clientRequest{
		Filters: clientFilters{
			Active: true,
		},
		Sorts:                 map[string]any{},
		HideHealthUnsupported: true,
		Page:                  1,
		PageSize:              1000,
		Scope:                 1,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")

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

// clientResponse represents the Open API response for network clients.
type clientResponse struct {
	Result struct {
		Data []model.NetworkClient `json:"data"`
	} `json:"result"`
}

// clientRequest represents the Open API request payload for network clients.
type clientRequest struct {
	Filters               clientFilters  `json:"filters"`
	Sorts                 map[string]any `json:"sorts"`
	HideHealthUnsupported bool           `json:"hideHealthUnsupported"`
	Page                  int            `json:"page"`
	PageSize              int            `json:"pageSize"`
	Scope                 int            `json:"scope"`
}

// clientFilters stores filters used in network client Open API requests.
type clientFilters struct {
	Active bool `json:"active"`
}
