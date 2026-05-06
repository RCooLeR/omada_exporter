package openapi

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
)

// GetVpnStats returns cached VPN tunnel statistics loaded from the Open API.
func (c *Client) GetVpnStats() ([]model.VpnStats, error) {
	return api.FetchCached(c.Client, "openapi:vpnstats", c.getVpnStatsFresh)
}

// getVpnStatsFresh fetches VPN tunnel statistics from the Open API and decodes
// the current site's tunnel metrics into VpnStats records.
func (c *Client) getVpnStatsFresh() ([]model.VpnStats, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/tunnel?page=1&pageSize=1000", c.Config.Host, c.OmadaCID, c.SiteId)
	vpnstatsdata := VpnStatsResponse{}
	if err := c.getOpenAPIJSON(url, "VPNStats", &vpnstatsdata); err != nil {
		return nil, err
	}

	return vpnstatsdata.Result.Data, nil
}

// VpnStatsResponse represents the Open API response for VPN statistics.
type VpnStatsResponse struct {
	Result struct {
		Data []model.VpnStats `json:"data"`
	} `json:"result"`
}
