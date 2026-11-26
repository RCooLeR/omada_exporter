package config

import (
	"errors"
	"strings"
)

const (
	// DefaultMSPDiscoveryInterval is how often MSP customers and sites are re-listed, in seconds.
	DefaultMSPDiscoveryInterval = 600
	// DefaultMSPScrapeConcurrency is how many MSP targets are scraped at the same time.
	DefaultMSPScrapeConcurrency = 4
)

// ValidateMSP reports configuration that cannot work against an Omada
// controller in MSP mode. It is checked before startup when MSP mode is
// requested explicitly, and again after auto-detection.
//
// MSP mode requires OpenAPI client credentials of an MSP-level application:
// web-session OpenAPI (as used for Fusion) is rejected by MSP controllers with
// HTTP 401 / -44116, and customer and site discovery needs the OpenAPI.
func ValidateMSP(c *Config) error {
	if c == nil {
		return errors.New("exporter configuration is nil")
	}
	var problems []error
	if strings.TrimSpace(c.ClientId) == "" || strings.TrimSpace(c.SecretId) == "" {
		problems = append(problems, errors.New("MSP mode requires OMADA_CLIENT_ID and OMADA_SECRET_ID of an MSP-level Open API application (Client mode)"))
	}
	switch strings.ToLower(strings.TrimSpace(c.OpenAPIAuth)) {
	case "", OpenAPIAuthAuto, OpenAPIAuthClientCredentials:
	default:
		problems = append(problems, errors.New("MSP mode requires OMADA_OPENAPI_AUTH=auto or client_credentials; the MSP controller rejects web-session OpenAPI and customer discovery needs the OpenAPI"))
	}
	if c.MQTTEnabled {
		problems = append(problems, errors.New("the Home Assistant MQTT publisher is not supported in MSP multi-customer mode; unset OMADA_MQTT_ENABLED or run a separate non-MSP exporter per site"))
	}
	if c.AllowDeviceControl {
		problems = append(problems, errors.New("device control is not supported in MSP multi-customer mode; unset OMADA_ALLOW_DEVICE_CONTROL"))
	}
	if c.MSPScrapeConcurrency < 0 {
		problems = append(problems, errors.New("OMADA_MSP_SCRAPE_CONCURRENCY must not be negative"))
	}
	if c.MSPDiscoveryInterval < 0 {
		problems = append(problems, errors.New("OMADA_MSP_DISCOVERY_INTERVAL must not be negative"))
	}
	return errors.Join(problems...)
}

// MSPCustomerFilter returns the configured customer names or IDs. An empty
// result means every customer.
func (c *Config) MSPCustomerFilter() []string {
	return parseMSPFilter(c.MSPCustomers, "all")
}

// MSPSiteFilter returns the configured site names or IDs for MSP mode. An empty
// result means every site of every selected customer. The flag default
// ("Default") is only applied when OMADA_SITE was set explicitly, because a
// single-site default makes no sense across many customers.
func (c *Config) MSPSiteFilter() []string {
	if !c.SiteSet {
		return nil
	}
	return parseMSPFilter(c.Site, "*")
}

func parseMSPFilter(value, wildcard string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, wildcard) {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.EqualFold(part, wildcard) {
			return nil
		}
		out = append(out, part)
	}
	return out
}
