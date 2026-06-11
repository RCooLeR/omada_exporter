package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"
)

// IsLoggedIn reports whether the current web session is still authenticated.
func (c *Client) IsLoggedIn() (bool, error) {
	url := fmt.Sprintf("%s/%s/api/v2/loginStatus", c.Config.Host, c.OmadaCID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	res, err := c.makeRequest(req)
	if err != nil {
		return false, err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return false, err
	}

	loginstatus := loginStatus{}
	err = json.Unmarshal(body, &loginstatus)
	if loginstatus.ErrorCode == -1200 {
		return false, nil
	}
	if loginstatus.ErrorCode != 0 {
		return false, fmt.Errorf("invalid error code returned from API. Response Body: %s", string(body))
	}

	return loginstatus.Result.Login, err
}

// getCid fetches the controller ID required for login requests.
func (c *Client) getCid() (string, error) {
	url := fmt.Sprintf("%s/api/info", c.Config.Host)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	res, err := c.makeRequest(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	var infoResponse struct {
		ErrorCode int    `json:"errorCode"`
		Msg       string `json:"msg"`
		Result    struct {
			OmadaCID string `json:"omadacId"`
		}
	}
	err = json.NewDecoder(res.Body).Decode(&infoResponse)
	if err != nil {
		return "", err
	}

	if infoResponse.Result.OmadaCID == "" {
		return "", fmt.Errorf("no CID found in response")
	}

	return infoResponse.Result.OmadaCID, nil
}

// Login authenticates the web session against the Omada controller.
func (c *Client) Login() error {

	url := fmt.Sprintf("%s/%s/api/v2/login", c.Config.Host, c.OmadaCID)
	jsonStr := []byte(fmt.Sprintf(`{"username":"%s","password":"%s"}`, c.Config.Username, c.Config.Password))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json; charset=UTF-8")
	res, err := c.makeRequest(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	logindata := loginResponse{}
	err = json.Unmarshal(body, &logindata)
	if err != nil {
		return err
	}
	if logindata.ErrorCode != 0 {
		return fmt.Errorf("web login failed with error code %d: %s", logindata.ErrorCode, logindata.Msg)
	}
	if logindata.Result.Token == "" {
		return fmt.Errorf("web login succeeded without a token")
	}
	log.Info().Msg(fmt.Sprintf("Login with username %s successful", c.Config.Username))
	c.setWebToken(logindata.Result.Token)
	return nil
}

// LoginOpenApi authenticates against the Omada Open API.
func (c *Client) LoginOpenApi() error {

	url := fmt.Sprintf("%s/openapi/authorize/token?grant_type=client_credentials", c.Config.Host)
	jsonStr := []byte(fmt.Sprintf(`{"omadacId":"%s","client_id":"%s","client_secret":"%s"}`, c.OmadaCID, c.Config.ClientId, c.Config.SecretId))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json; charset=UTF-8")
	res, err := c.makeRequest(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	logindata := loginResponse{}
	err = json.Unmarshal(body, &logindata)
	if err != nil {
		return err
	}
	if logindata.ErrorCode != 0 {
		return fmt.Errorf("OpenApi authentication failed with error code %d: %s", logindata.ErrorCode, logindata.Msg)
	}
	if logindata.Result.AccessToken == "" {
		return fmt.Errorf("OpenApi authentication succeeded without an access token")
	}
	log.Info().Msg("OpenApi authentication successful")
	c.setOpenAPITokens(logindata.Result.AccessToken, logindata.Result.RefreshToken, logindata.Result.ExpiresIn)
	return nil
}

// RefreshOpenApiToken refreshes the Open API access token.
func (c *Client) RefreshOpenApiToken() error {
	_, refreshToken, _ := c.currentOpenAPITokenState()
	if refreshToken == "" {
		return fmt.Errorf("OpenApi token refresh requested without a refresh token")
	}

	query := url.Values{}
	query.Set("client_id", c.Config.ClientId)
	query.Set("client_secret", c.Config.SecretId)
	query.Set("refresh_token", refreshToken)
	query.Set("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/openapi/authorize/token?%s", c.Config.Host, query.Encode()), nil)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json; charset=UTF-8")
	res, err := c.makeRequest(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	logindata := loginResponse{}

	err = json.Unmarshal(body, &logindata)
	if err != nil {
		return err
	}
	if logindata.ErrorCode != 0 {
		return fmt.Errorf("OpenApi token refresh failed with error code %d: %s", logindata.ErrorCode, logindata.Msg)
	}
	if logindata.Result.AccessToken == "" {
		return fmt.Errorf("OpenApi token refresh succeeded without an access token")
	}
	log.Info().Msg("OpenApi Access Token refresh successful")
	c.setOpenAPITokens(logindata.Result.AccessToken, logindata.Result.RefreshToken, logindata.Result.ExpiresIn)
	return nil
}

// loginResponse represents the API response for login.
type loginResponse struct {
	ErrorCode int    `json:"errorCode"`
	Msg       string `json:"msg"`
	Result    struct {
		Token        string `json:"token"`
		AccessToken  string `json:"accessToken"`
		TokenType    string `json:"tokenType"`
		ExpiresIn    int32  `json:"expiresIn"`
		RefreshToken string `json:"refreshToken"`
	} `json:"result"`
}

// loginStatus stores login status data.
type loginStatus struct {
	ErrorCode int `json:"errorCode"`
	Result    struct {
		Login bool `json:"login"`
	} `json:"result"`
}

// RefreshOmadaContext refreshes cached Omada session and site context data.
func (c *Client) RefreshOmadaContext() error {
	cid, err := c.getCid()
	if err != nil {
		return err
	}
	c.OmadaCID = cid
	return nil
}
