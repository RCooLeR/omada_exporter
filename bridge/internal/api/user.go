package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// getSiteId returns the site identifier for the configured site name.
func (c *Client) getSiteId(name string) (*string, error) {
	return c.getSiteIdWithRequest(name, c.MakeLoggedInRequest)
}

// getSiteIdFromCurrentSession returns the site identifier from the current session data.
func (c *Client) getSiteIdFromCurrentSession(name string) (*string, error) {
	return c.getSiteIdWithRequest(name, c.makeRequest)
}

// getSiteIdWithRequest resolves the site identifier using the provided request function.
func (c *Client) getSiteIdWithRequest(name string, requestFn func(*http.Request) (*http.Response, error)) (*string, error) {
	url := fmt.Sprintf("%s/%s/api/v2/users/current", c.Config.Host, c.OmadaCID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := requestFn(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
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

	return nil, fmt.Errorf("failed to find site with name %s", name)
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
