package omada

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

// GetAPUplinkStats returns the AP's cached wired-uplink packet counters. A nil
// result means the controller supplied no counters, for example for a wireless
// backhaul AP. Unsupported endpoints and invalid responses remain errors.
func (c *Client) GetAPUplinkStats(mac string) (*model.APUplinkStats, error) {
	if strings.TrimSpace(mac) == "" {
		return nil, fmt.Errorf("AP MAC address is required for wired uplink statistics")
	}
	return c.session.FetchCached("openapi:ap:wired-uplink:"+mac, func() (*model.APUplinkStats, error) {
		return c.getAPUplinkStatsFresh(mac)
	})
}

func (c *Client) getAPUplinkStatsFresh(mac string) (*model.APUplinkStats, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}
	omadaCID, siteID := c.ContextIDs()
	endpoint := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/wired-uplink", c.session.Config.Host, omadaCID, siteID, url.PathEscape(mac))
	var response struct {
		Result struct {
			WiredUplink *model.APUplinkStats `json:"wiredUplink"`
		} `json:"result"`
	}
	if err := c.getOpenAPIJSON(endpoint, "AP wired uplink statistics", &response); err != nil {
		return nil, err
	}
	if !response.Result.WiredUplink.HasCounters() {
		return nil, nil
	}
	return response.Result.WiredUplink, nil
}
