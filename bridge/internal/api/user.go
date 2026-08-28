package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
)

// getSiteId returns the site identifier for the configured site name.
func (c *Client) getSiteId(name string) (*string, error) {
	omadaCID, _ := c.ContextIDs()
	return c.getSiteIdWithRequest(name, omadaCID, c.MakeLoggedInRequest)
}

func (c *Client) getSiteIdFromCurrentSessionForCID(name, omadaCID string) (*string, error) {
	return c.getSiteIdWithRequest(name, omadaCID, c.makeRequest)
}

// getSiteIdWithRequest resolves the site identifier using the provided request function.
func (c *Client) getSiteIdWithRequest(name, omadaCID string, requestFn func(*http.Request) (*http.Response, error)) (*string, error) {
	url := fmt.Sprintf("%s/%s/api/v2/users/current", c.Config.Host, omadaCID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := requestFn(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ReadResponseBody(resp, "current user")
	if err != nil {
		return nil, err
	}
	if err := ValidateAPIResponse(body, "current user"); err != nil {
		return nil, err
	}

	user := userResponse{}
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, err
	}

	for _, s := range user.Result.Privilege.Sites {
		if s.Key == name {
			return &s.Value, nil
		}
	}

	if len(user.Result.Privilege.Sites) == 1 && c.canUseSingleSiteFallback(name) {
		site := user.Result.Privilege.Sites[0]
		log.Warn().
			Str("requested_site", name).
			Str("selected_site", site.Key).
			Msg("configured site was not found; using the only available Omada site")
		return &site.Value, nil
	}

	return nil, fmt.Errorf("failed to find site with name %s; available sites: %s", name, strings.Join(user.siteNames(), ", "))
}

func (c *Client) canUseSingleSiteFallback(name string) bool {
	systemType := normalizeOption(c.Config.SystemType, config.SystemTypeAuto)
	if systemType != config.SystemTypeAuto && systemType != config.SystemTypeFusion {
		return false
	}
	name = strings.TrimSpace(name)
	return name == "" || strings.EqualFold(name, "Default")
}

func (u *userResponse) siteNames() []string {
	names := make([]string, 0, len(u.Result.Privilege.Sites))
	for _, s := range u.Result.Privilege.Sites {
		names = append(names, s.Key)
	}
	return names
}

// userResponse represents the API response for user.
type userResponse struct {
	Result struct {
		Privilege struct {
			Sites []struct {
				Key   string `json:"name"`
				Value string `json:"key"`
			} `json:"sites"`
		} `json:"privilege"`
	} `json:"result"`
}
