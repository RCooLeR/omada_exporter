package openapi

import (
	"github.com/RCooLeR/omada_exporter/internal/api"
)

// hack for keeping logic in separate dirs
type Client struct {
	*api.Client
}
