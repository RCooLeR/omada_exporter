package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	log "github.com/rs/zerolog/log"
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

// GetSiteToSiteVpnSummaries returns site-to-site VPN summary data.
func (c *Client) GetSiteToSiteVpnSummaries() ([]model.SiteToSiteVpnSummary, error) {
	return api.FetchCached(c.Client, "openapi:vpn:s2s:summary", c.getSiteToSiteVpnSummariesFresh)
}

// getSiteToSiteVpnSummariesFresh fetches fresh site-to-site VPN summary data from the Open API.
func (c *Client) getSiteToSiteVpnSummariesFresh() ([]model.SiteToSiteVpnSummary, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	urlTemplate := fmt.Sprintf("%s/openapi/v2/%s/sites/%s/vpn/site-to-site-vpns?page=%%d&pageSize=%%d", c.Config.Host, c.OmadaCID, c.SiteId)
	return fetchOpenAPIGrid[model.SiteToSiteVpnSummary](c, "site-to-site VPN summary", urlTemplate)
}

// GetSiteToSiteVpnStats returns site-to-site VPN statistics.
func (c *Client) GetSiteToSiteVpnStats() ([]model.SiteToSiteVpnStats, error) {
	return api.FetchCached(c.Client, "openapi:vpn:s2s:stats", c.getSiteToSiteVpnStatsFresh)
}

// getSiteToSiteVpnStatsFresh fetches fresh site-to-site VPN statistics from the Open API.
func (c *Client) getSiteToSiteVpnStatsFresh() ([]model.SiteToSiteVpnStats, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	var all []model.SiteToSiteVpnStats
	for _, vpnType := range []int{2, 4} {
		urlTemplate := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/s2s?filters.vpnType=%d&page=%%d&pageSize=%%d", c.Config.Host, c.OmadaCID, c.SiteId, vpnType)
		items, err := fetchOpenAPIGrid[model.SiteToSiteVpnStats](c, fmt.Sprintf("site-to-site VPN stats type=%d", vpnType), urlTemplate)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}

	return all, nil
}

// GetSiteToSiteVpnPeerStats returns peer statistics for a site-to-site VPN.
func (c *Client) GetSiteToSiteVpnPeerStats(vpnID string) ([]model.SiteToSiteVpnPeerStats, error) {
	cacheKey := fmt.Sprintf("openapi:vpn:s2s:peer:%s", vpnID)
	return api.FetchCached(c.Client, cacheKey, func() ([]model.SiteToSiteVpnPeerStats, error) {
		return c.getSiteToSiteVpnPeerStatsFresh(vpnID)
	})
}

// getSiteToSiteVpnPeerStatsFresh fetches fresh peer statistics for a site-to-site VPN.
func (c *Client) getSiteToSiteVpnPeerStatsFresh(vpnID string) ([]model.SiteToSiteVpnPeerStats, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	urlTemplate := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/s2s/%s/peer?page=%%d&pageSize=%%d", c.Config.Host, c.OmadaCID, c.SiteId, vpnID)
	return fetchOpenAPIGrid[model.SiteToSiteVpnPeerStats](c, fmt.Sprintf("site-to-site VPN peer stats vpn=%s", vpnID), urlTemplate)
}

// requireOpenAPICredentials validates that Open API credentials are configured.
func (c *Client) requireOpenAPICredentials() error {
	if c.Config.ClientId == "" || c.Config.SecretId == "" {
		return fmt.Errorf("ClientId and SecretId are required parameters.")
	}
	return nil
}

// fetchOpenAPIGrid loads every page from a paginated Open API grid endpoint.
func fetchOpenAPIGrid[T any](client *Client, endpointName, urlTemplate string) ([]T, error) {
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

	resp, err := c.MakeOpenApiRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	log.Info().Msgf("Received data from %s endpoint", endpointName)
	log.Debug().Bytes("data", body).Msgf("Received data from %s endpoint", endpointName)

	return json.Unmarshal(body, target)
}
