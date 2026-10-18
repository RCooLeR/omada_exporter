package omada

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

// GetSiteToSiteVpnSummaries returns site-to-site VPN summary data.
func (c *Client) GetSiteToSiteVpnSummaries() ([]model.SiteToSiteVpnSummary, error) {
	return c.session.FetchCached("openapi:vpn:s2s:summary", c.getSiteToSiteVpnSummariesFresh)
}

// getSiteToSiteVpnSummariesFresh fetches fresh site-to-site VPN summary data from the Open API.
func (c *Client) getSiteToSiteVpnSummariesFresh() ([]model.SiteToSiteVpnSummary, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	omadaCID, siteID := c.ContextIDs()
	urlTemplate := fmt.Sprintf("%s/openapi/v2/%s/sites/%s/vpn/site-to-site-vpns?page=%%d&pageSize=%%d", c.session.Config.Host, omadaCID, siteID)
	return c.fetchOpenAPIGrid[model.SiteToSiteVpnSummary]("site-to-site VPN summary", urlTemplate)
}

// GetSiteToSiteVpnStats returns site-to-site VPN statistics.
func (c *Client) GetSiteToSiteVpnStats() ([]model.SiteToSiteVpnStats, error) {
	return c.session.FetchCached("openapi:vpn:s2s:stats", c.getSiteToSiteVpnStatsFresh)
}

// getSiteToSiteVpnStatsFresh fetches fresh site-to-site VPN statistics from the Open API.
func (c *Client) getSiteToSiteVpnStatsFresh() ([]model.SiteToSiteVpnStats, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	var all []model.SiteToSiteVpnStats
	omadaCID, siteID := c.ContextIDs()
	for _, vpnType := range []int{2, 4} {
		urlTemplate := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/s2s?filters.vpnType=%d&page=%%d&pageSize=%%d", c.session.Config.Host, omadaCID, siteID, vpnType)
		items, err := c.fetchOpenAPIGrid[model.SiteToSiteVpnStats](fmt.Sprintf("site-to-site VPN stats type=%d", vpnType), urlTemplate)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}

	return all, nil
}

// GetSiteToSiteVpnPeerStats returns peer statistics for a site-to-site VPN tunnel.
func (c *Client) GetSiteToSiteVpnPeerStats(tunnelID string) ([]model.SiteToSiteVpnPeerStats, error) {
	cacheKey := fmt.Sprintf("openapi:vpn:s2s:peer:%s", tunnelID)
	return c.session.FetchCached(cacheKey, func() ([]model.SiteToSiteVpnPeerStats, error) {
		return c.getSiteToSiteVpnPeerStatsFresh(tunnelID)
	})
}

// getSiteToSiteVpnPeerStatsFresh fetches fresh peer statistics for a site-to-site VPN tunnel.
func (c *Client) getSiteToSiteVpnPeerStatsFresh(tunnelID string) ([]model.SiteToSiteVpnPeerStats, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}

	omadaCID, siteID := c.ContextIDs()
	urlTemplate := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/s2s/%s/peer?page=%%d&pageSize=%%d", c.session.Config.Host, omadaCID, siteID, tunnelID)
	return c.fetchOpenAPIGrid[model.SiteToSiteVpnPeerStats](fmt.Sprintf("site-to-site VPN peer stats tunnel=%s", tunnelID), urlTemplate)
}
