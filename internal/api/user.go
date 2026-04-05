package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// there's no nice way of fetching the site ID from the `Viewer` role
// calling the user endpoint seems to return a list of sites for the user
func (c *Client) getSiteId(name string) (*string, error) {
	return c.getSiteIdWithRequest(name, c.MakeLoggedInRequest)
}

func (c *Client) getSiteIdFromCurrentSession(name string) (*string, error) {
	return c.getSiteIdWithRequest(name, c.makeRequest)
}

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
