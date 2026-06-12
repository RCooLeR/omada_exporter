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

	urlTemplate := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/tunnel?page=%%d&pageSize=%%d", c.Config.Host, c.OmadaCID, c.SiteId)
	return fetchOpenAPIGrid[model.VpnStats](c, "VPNStats", urlTemplate)
}
