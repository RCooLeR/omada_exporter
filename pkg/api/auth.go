package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

func (c *Client) IsLoggedIn() (bool, error) {
	loginstatus := loginStatus{}

	url := fmt.Sprintf("%s/%s/api/v2/loginStatus", c.Config.Host, c.omadaCID)
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

	err = json.Unmarshal(body, &loginstatus)
	if loginstatus.ErrorCode == -1200 {
		return false, nil
	}
	if loginstatus.ErrorCode != 0 {
		return false, fmt.Errorf("invalid error code returned from API. Response Body: %s", string(body))
	}

	return loginstatus.Result.Login, err
}

// one of the "quirks" of the omada API - it requires a CID to be part of the path
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

func (c *Client) Login() error {
	logindata := loginResponse{}

	url := fmt.Sprintf("%s/%s/api/v2/login", c.Config.Host, c.omadaCID)
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

	err = json.Unmarshal(body, &logindata)
	if err != nil {
		return err
	}
	log.Info().Msg(fmt.Sprintf("Login with username %s successful", c.Config.Username))
	c.token = logindata.Result.Token
	return nil
}

func (c *Client) LoginOpenApi() error {
	logindata := loginResponse{}

	url := fmt.Sprintf("%s/openapi/authorize/token?grant_type=client_credentials", c.Config.Host)
	jsonStr := []byte(fmt.Sprintf(`{"omadacId":"%s","client_id":"%s","client_secret":"%s"}`, c.omadaCID, c.Config.ClientId, c.Config.SecretId))
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

	err = json.Unmarshal(body, &logindata)
	if err != nil {
		return err
	}
	log.Info().Msg("OpenApi authentication successful")
	c.accessToken = logindata.Result.AccessToken
	c.refreshToken = logindata.Result.RefreshToken
	c.accessTokenExpiresAt = time.Now().Add(time.Duration(logindata.Result.ExpiresIn-5) * time.Second)
	return nil
}
func (c *Client) RefreshOpenApiToken() error {
	url := fmt.Sprintf("%s/openapi/authorize/token?client_id=%s&client_secret=%s&refresh_token=%s&grant_type=refresh_token", c.Config.Host, c.Config.ClientId, c.Config.SecretId, c.refreshToken)
	req, err := http.NewRequest("POST", url, nil)
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
	log.Info().Msg("OpenApi Access Token refresh successful")
	c.accessToken = logindata.Result.AccessToken
	c.refreshToken = logindata.Result.RefreshToken
	c.accessTokenExpiresAt = time.Now().Add(time.Duration(logindata.Result.ExpiresIn-5) * time.Second)
	return nil
}

type loginResponse struct {
	Result loginResult `json:"result"`
}
type loginResult struct {
	Token        string `json:"token"`
	AccessToken  string `json:"accessToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int32  `json:"expiresIn"`
	RefreshToken string `json:"refreshToken"`
}
type loginStatus struct {
	ErrorCode int            `json:"errorCode"`
	Result    loggedInResult `json:"result"`
}
type loggedInResult struct {
	Login bool `json:"login"`
}
