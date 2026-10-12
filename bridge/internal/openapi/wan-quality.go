package openapi

import (
	"fmt"
	"math"
	"net/url"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

// GetWANQuality returns gateway-wide WAN quality averages over the requested
// rolling window. A successful response without supported measurements returns
// nil; request/authentication/API failures remain errors for collector health.
func (c *Client) GetWANQuality(mac string, windowSeconds int) (*model.WANQuality, error) {
	if windowSeconds <= 0 || int64(windowSeconds) > math.MaxInt64/1000 {
		return nil, fmt.Errorf("WAN quality window must be a positive number of seconds within the supported timestamp range")
	}
	if mac == "" {
		return nil, fmt.Errorf("WAN quality requires a gateway MAC")
	}
	key := fmt.Sprintf("openapi:wan-quality:%s:%d", mac, windowSeconds)
	return c.FetchCached(key, func() (*model.WANQuality, error) {
		return c.getWANQualityFresh(mac, windowSeconds)
	})
}

func (c *Client) getWANQualityFresh(mac string, windowSeconds int) (*model.WANQuality, error) {
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}
	omadaCID, siteID := c.ContextIDs()
	end := time.Now().UnixMilli()
	start := end - int64(windowSeconds)*1000
	endpoint := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/health/detail?start=%d&end=%d",
		c.Config.Host, omadaCID, siteID, url.PathEscape(mac), start, end)
	var response wanQualityResponse
	if err := c.getOpenAPIJSON(endpoint, "WAN quality", &response); err != nil {
		return nil, err
	}
	if response.Result == nil || response.Result.WANStatus == nil || !response.Result.WANStatus.Support {
		return nil, nil
	}
	status := response.Result.WANStatus
	quality := &model.WANQuality{
		LatencyMilliseconds: nonNegativeWANMeasurement(status.AverageLatency),
		JitterMilliseconds:  nonNegativeWANMeasurement(status.AverageJitter),
		PacketLossPercent:   nonNegativeWANMeasurement(status.AveragePacketLoss),
		MOS:                 nonNegativeWANMeasurement(status.AverageMOS),
	}
	if quality.LatencyMilliseconds == nil && quality.JitterMilliseconds == nil && quality.PacketLossPercent == nil && quality.MOS == nil {
		return nil, nil
	}
	return quality, nil
}

// Controller health data uses negative values as unavailable sentinels. Preserve
// genuine zero latency, jitter, and loss rather than turning missing data into 0.
func nonNegativeWANMeasurement(value *float64) *float64 {
	if value == nil || *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	return value
}

type wanQualityResponse struct {
	Result *struct {
		WANStatus *struct {
			Support           bool     `json:"support"`
			AverageLatency    *float64 `json:"averageLatency"`
			AverageJitter     *float64 `json:"averageJitter"`
			AveragePacketLoss *float64 `json:"averagePktLoss"`
			AverageMOS        *float64 `json:"averageMos"`
		} `json:"wanStatus"`
	} `json:"result"`
}
