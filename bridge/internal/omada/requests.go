package omada

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
	"github.com/rs/zerolog/log"
)

const openAPIPageSize = 100

// openAPIGridResponse represents a paginated Open API grid response.
type openAPIGridResponse[T any] struct {
	Result struct {
		TotalRows   int `json:"totalRows"`
		CurrentPage int `json:"currentPage"`
		CurrentSize int `json:"currentSize"`
		Data        []T `json:"data"`
	} `json:"result"`
}

// requireOpenAPICredentials validates that Open API credentials are configured.
func (c *Client) requireOpenAPICredentials() error {
	if c.session.OpenAPIAuthMode == config.OpenAPIAuthDisabled {
		return fmt.Errorf("OpenAPI authentication is disabled or unavailable")
	}
	if c.session.OpenAPIAuthMode == config.OpenAPIAuthClientCredentials && (c.session.Config.ClientId == "" || c.session.Config.SecretId == "") {
		return fmt.Errorf("ClientId and SecretId are required for OpenAPI client-credentials authentication")
	}
	return nil
}

// fetchOpenAPIGrid loads every page from a paginated Open API grid endpoint.
func (client *Client) fetchOpenAPIGrid[T any](endpointName, urlTemplate string) ([]T, error) {
	var all []T

	for page := 1; ; page++ {
		url := fmt.Sprintf(urlTemplate, page, openAPIPageSize)
		response := openAPIGridResponse[T]{}
		if err := client.getOpenAPIJSON(url, endpointName, &response); err != nil {
			return nil, err
		}

		all = append(all, response.Result.Data...)

		totalRows := response.Result.TotalRows
		if totalRows <= 0 || len(response.Result.Data) == 0 || len(all) >= totalRows {
			return all, nil
		}
	}
}

// getOpenAPIJSON fetches an Open API endpoint and decodes its JSON response.
func (c *Client) getOpenAPIJSON(url, endpointName string, target any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error().Err(err).Msgf("Error creating GET request for %s", url)
		return err
	}

	resp, err := c.session.MakeOpenApiRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := session.ReadResponseBody(resp, endpointName)
	if err != nil {
		return err
	}

	log.Info().Msgf("Received data from %s endpoint", endpointName)
	log.Debug().Bytes("data", body).Msgf("Received data from %s endpoint", endpointName)

	if err := session.ValidateAPIResponse(body, endpointName); err != nil {
		return err
	}

	return json.Unmarshal(body, target)
}

func (c *Client) getWebAPIJSON(url, endpointName string, target any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.session.MakeLoggedInRequest(req)
	if err != nil {
		return err
	}

	body, err := session.ReadResponseBody(resp, endpointName)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}

	log.Info().Msgf("Received data from %s endpoint", endpointName)
	log.Debug().Bytes("data", body).Msgf("Received data from %s endpoint", endpointName)

	if err := session.ValidateAPIResponse(body, endpointName); err != nil {
		return err
	}

	return json.Unmarshal(body, target)
}
