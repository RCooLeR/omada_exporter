package openapi

import (
	"github.com/RCooLeR/omada_exporter/internal/api"
)

// Client wraps the shared API client for Omada Open API calls.
type Client struct {
	*api.Client
}
