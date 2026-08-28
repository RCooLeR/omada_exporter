package openapi

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

// GetVpnStats returns cached VPN tunnel statistics loaded from the Open API.
func (c *Client) GetVpnStats() ([]model.VpnStats, error) {
	return c.FetchCached("openapi:vpnstats", c.getVpnStatsFresh)
}

// getVpnStatsFresh fetches VPN tunnel statistics from the Open API and decodes
// the current site's tunnel metrics into VpnStats records.
func (c *Client) getVpnStatsFresh() ([]model.VpnStats, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	omadaCID, siteID := c.ContextIDs()
	urlTemplate := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/tunnel?page=%%d&pageSize=%%d", c.Config.Host, omadaCID, siteID)
	return c.fetchOpenAPIGrid[model.VpnStats]("VPNStats", urlTemplate)
}
