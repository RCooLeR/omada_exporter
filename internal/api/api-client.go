package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
)

type Client struct {
	Config               *config.Config
	httpClient           *http.Client
	token                string
	OmadaCID             string
	SiteId               string
	accessToken          string
	refreshToken         string
	accessTokenExpiresAt time.Time
}

func createHttpClient(insecure bool, timeout int) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to init cookiejar")
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxConnsPerHost = 100
	t.MaxIdleConnsPerHost = 100

	client := &http.Client{Transport: t, Timeout: time.Duration(timeout) * time.Second, Jar: jar}

	if insecure {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return client, nil
}

func Configure(c *config.Config) (*Client, error) {
	httpClient, err := createHttpClient(c.Insecure, c.Timeout)
	if err != nil {
		return nil, err
	}

	client := &Client{
		Config:     c,
		httpClient: httpClient,
	}
	cid, err := client.getCid()
	if err != nil {
		return nil, err
	}
	client.OmadaCID = cid

	sid, err := client.getSiteId(c.Site)
	if err != nil {
		return nil, err
	}
	client.SiteId = *sid
	if client.Config.ClientId != "" && client.Config.SecretId != "" {
		err := client.LoginOpenApi()
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

func (c *Client) makeRequest(req *http.Request) (*http.Response, error) {
	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-Requested-With", "XMLHttpRequest")
	req.Header.Add("User-Agent", "omada_exporter")
	req.Header.Add("Connection", "keep-alive")

	if c.token != "" {
		req.Header.Add("Csrf-Token", c.token)
	}

	return c.httpClient.Do(req)
}

func (c *Client) MakeLoggedInRequest(req *http.Request) (*http.Response, error) {
	loggedIn, err := c.IsLoggedIn()
	if err != nil {
		return nil, err
	}
	if !loggedIn {
		log.Info().Msg(fmt.Sprintf("not logged in, logging in with user: %s", c.Config.Username))
		err := c.Login()
		if err != nil || c.token == "" {
			log.Error().Err(err).Msg("failed to login")
			return nil, err
		}
	}
	log.Info().Msg(fmt.Sprintf("MakeLoggedInRequest %s", req.URL.String()))

	return c.makeRequest(req)
}

func (c *Client) MakeOpenApiRequest(req *http.Request) (*http.Response, error) {
	//	with bearer token accessToken
	if time.Now().After(c.accessTokenExpiresAt) && (c.refreshToken != "") {
		err := c.RefreshOpenApiToken()
		if err != nil {
			return nil, err
		}
	}
	if c.accessTokenExpiresAt.IsZero() || time.Now().After(c.accessTokenExpiresAt) || (c.accessToken == "") {
		err := c.LoginOpenApi()
		if err != nil {
			return nil, err
		}
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-Requested-With", "XMLHttpRequest")
	req.Header.Add("User-Agent", "omada_exporter")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("Authorization", "AccessToken="+c.accessToken)

	return c.httpClient.Do(req)
}
