// Package mspfake provides an httptest Omada controller in MSP mode for tests.
//
// It reproduces the behaviour probed against a real controller (v5.15.24.19,
// mspMode=true): authentication only works with the MSP ID, data requests only
// work with a customer ID, and the controller answers with the same error codes
// (-1, -1005, -44106, -44116) where the real one does.
package mspfake

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	sessionCookie = "TPOMADA_SESSIONID"

	// Error codes observed on the real controller.
	CodeGeneralError        = -1
	CodeOperationForbidden  = -1005
	CodeLoginRequired       = -1200
	CodeInvalidClient       = -44106
	CodeTokenExpired        = -44112
	CodeOpenAPIUnauthorized = -44116
)

// Site is one site of a customer.
type Site struct {
	ID   string
	Name string
}

// Customer is one MSP customer.
type Customer struct {
	ID    string
	Name  string
	Sites []Site
}

// Request is a recorded request.
type Request struct {
	Method         string
	Path           string
	Query          string
	Body           string
	HasAccessToken bool
	HasCsrfToken   bool
	HasSession     bool
}

// Controller is a fake MSP-mode Omada controller.
type Controller struct {
	MSPID        string
	MSPName      string
	Username     string
	Password     string
	ClientID     string
	ClientSecret string
	// MaxPageSize caps OpenAPI page sizes so tests exercise pagination.
	MaxPageSize int

	Server *httptest.Server

	mu               sync.Mutex
	reportMSPMode    bool
	customers        []Customer
	forbidden        map[string]bool
	failCustomerList bool
	failSiteList     map[string]bool
	sessions         map[string]string // cookie -> csrf token
	tokens           map[string]bool
	refreshTokens    map[string]bool
	serial           int
	requests         []Request
	webLogins        int
	openAPILogins    int
}

// DefaultCustomers returns three customers with four sites in total.
func DefaultCustomers() []Customer {
	return []Customer{
		{ID: "cust-a", Name: "Alpha Corp", Sites: []Site{{ID: "site-a1", Name: "Default"}, {ID: "site-a2", Name: "Branch"}}},
		{ID: "cust-b", Name: "Beta Ltd", Sites: []Site{{ID: "site-b1", Name: "Default"}}},
		{ID: "cust-c", Name: "Gamma", Sites: []Site{{ID: "site-c1", Name: "HQ"}}},
	}
}

// New starts a fake controller with DefaultCustomers and a page size of 2.
func New(t testing.TB) *Controller {
	t.Helper()
	c := &Controller{
		MSPID:         "msp-1",
		MSPName:       "Example MSP",
		Username:      "msp-owner",
		Password:      "msp-password",
		ClientID:      "msp-client",
		ClientSecret:  "msp-secret",
		MaxPageSize:   2,
		reportMSPMode: true,
		customers:     DefaultCustomers(),
		forbidden:     map[string]bool{},
		failSiteList:  map[string]bool{},
		sessions:      map[string]string{},
		tokens:        map[string]bool{},
		refreshTokens: map[string]bool{},
	}
	c.Server = httptest.NewServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.Server.Close)
	return c
}

// URL returns the controller base URL.
func (c *Controller) URL() string { return c.Server.URL }

// SetReportMSPMode controls the mspMode flag in /api/info.
func (c *Controller) SetReportMSPMode(v bool) {
	c.mu.Lock()
	c.reportMSPMode = v
	c.mu.Unlock()
}

// SetCustomers replaces the customer list.
func (c *Controller) SetCustomers(customers []Customer) {
	c.mu.Lock()
	c.customers = customers
	c.mu.Unlock()
}

// SetForbidden makes every request for the customer fail with -1005.
func (c *Controller) SetForbidden(customerID string, v bool) {
	c.mu.Lock()
	c.forbidden[customerID] = v
	c.mu.Unlock()
}

// SetFailCustomerList makes the MSP customer listing fail.
func (c *Controller) SetFailCustomerList(v bool) {
	c.mu.Lock()
	c.failCustomerList = v
	c.mu.Unlock()
}

// SetFailSiteList makes the site listing of one customer fail.
func (c *Controller) SetFailSiteList(customerID string, v bool) {
	c.mu.Lock()
	c.failSiteList[customerID] = v
	c.mu.Unlock()
}

// ExpireWebSessions invalidates every web session.
func (c *Controller) ExpireWebSessions() {
	c.mu.Lock()
	c.sessions = map[string]string{}
	c.mu.Unlock()
}

// ExpireAccessTokens invalidates every OpenAPI access and refresh token.
func (c *Controller) ExpireAccessTokens() {
	c.mu.Lock()
	c.tokens = map[string]bool{}
	c.refreshTokens = map[string]bool{}
	c.mu.Unlock()
}

// Requests returns a copy of the recorded requests.
func (c *Controller) Requests() []Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Request(nil), c.requests...)
}

// ResetRequests clears the recorded requests.
func (c *Controller) ResetRequests() {
	c.mu.Lock()
	c.requests = nil
	c.mu.Unlock()
}

// Logins returns the number of successful web and OpenAPI logins.
func (c *Controller) Logins() (web, openAPI int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.webLogins, c.openAPILogins
}

func (c *Controller) customer(id string) (Customer, bool) {
	for _, customer := range c.customers {
		if customer.ID == id {
			return customer, true
		}
	}
	return Customer{}, false
}

func (c *Controller) nextID(prefix string) string {
	c.serial++
	return fmt.Sprintf("%s-%d", prefix, c.serial)
}

func (c *Controller) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()

	cookie, _ := r.Cookie(sessionCookie)
	hasSession := cookie != nil && c.sessions[cookie.Value] != ""
	csrf := r.Header.Get("Csrf-Token")
	accessToken, hasAccessToken := strings.CutPrefix(r.Header.Get("Authorization"), "AccessToken=")
	c.requests = append(c.requests, Request{
		Method:         r.Method,
		Path:           r.URL.Path,
		Query:          r.URL.RawQuery,
		Body:           string(body),
		HasAccessToken: hasAccessToken,
		HasCsrfToken:   csrf != "",
		HasSession:     hasSession,
	})
	webAuthed := hasSession && csrf != "" && c.sessions[cookie.Value] == csrf

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.URL.Path == "/api/info":
		writeOK(w, map[string]any{"omadacId": c.MSPID, "mspMode": c.reportMSPMode, "controllerVer": "5.15.24.19"})
	case r.URL.Path == "/openapi/authorize/token":
		c.serveToken(w, r, body)
	case len(parts) >= 2 && parts[0] == "openapi":
		c.serveOpenAPI(w, r, parts, hasAccessToken, accessToken)
	case len(parts) >= 3 && parts[1] == "api" && parts[2] == "v2":
		c.serveWeb(w, r, parts[0], strings.Join(parts[3:], "/"), body, webAuthed)
	default:
		writeError(w, http.StatusNotFound, CodeGeneralError, "Not found.")
	}
}

func (c *Controller) serveToken(w http.ResponseWriter, r *http.Request, body []byte) {
	query := r.URL.Query()
	switch query.Get("grant_type") {
	case "client_credentials":
		var request struct {
			OmadaCID     string `json:"omadacId"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		_ = json.Unmarshal(body, &request)
		// The app is MSP-level: it is only valid together with the MSP ID.
		if request.OmadaCID != c.MSPID || request.ClientID != c.ClientID || request.ClientSecret != c.ClientSecret {
			writeError(w, http.StatusOK, CodeInvalidClient, "Client Id Or Client Secret is Invalid.")
			return
		}
	case "refresh_token":
		if query.Get("client_id") != c.ClientID || query.Get("client_secret") != c.ClientSecret || !c.refreshTokens[query.Get("refresh_token")] {
			writeError(w, http.StatusOK, CodeInvalidClient, "Client Id Or Client Secret is Invalid.")
			return
		}
		delete(c.refreshTokens, query.Get("refresh_token"))
	default:
		writeError(w, http.StatusOK, CodeGeneralError, "General error.")
		return
	}
	token, refresh := c.nextID("access"), c.nextID("refresh")
	c.tokens[token] = true
	c.refreshTokens[refresh] = true
	c.openAPILogins++
	writeOK(w, map[string]any{"accessToken": token, "tokenType": "bearer", "expiresIn": 7200, "refreshToken": refresh})
}

func (c *Controller) serveOpenAPI(w http.ResponseWriter, r *http.Request, parts []string, hasAccessToken bool, accessToken string) {
	if !hasAccessToken {
		// Includes web-session OpenAPI (Csrf-Token without AccessToken, as
		// used for Fusion), which MSP controllers reject.
		writeError(w, http.StatusUnauthorized, CodeOpenAPIUnauthorized, "Unauthorized.")
		return
	}
	if !c.tokens[accessToken] {
		writeError(w, http.StatusOK, CodeTokenExpired, "The access token has expired.")
		return
	}
	if len(parts) < 3 {
		writeError(w, http.StatusNotFound, CodeGeneralError, "Not found.")
		return
	}
	// /openapi/v1/msp/{mspId}/customers
	if parts[2] == "msp" {
		if len(parts) == 5 && parts[3] == c.MSPID && parts[4] == "customers" {
			if c.failCustomerList {
				writeError(w, http.StatusOK, CodeGeneralError, "General error.")
				return
			}
			rows := make([]any, 0, len(c.customers))
			for _, customer := range c.customers {
				rows = append(rows, map[string]any{"customerId": customer.ID, "customerName": customer.Name})
			}
			c.writePage(w, r, rows)
			return
		}
		writeError(w, http.StatusOK, CodeOperationForbidden, "Operation forbidden.")
		return
	}
	cid := parts[2]
	customer, ok := c.customer(cid)
	if !ok || c.forbidden[cid] {
		// Includes /openapi/v1/{mspId}/...: the MSP ID is not a valid data scope.
		writeError(w, http.StatusOK, CodeOperationForbidden, "Operation forbidden.")
		return
	}
	rest := parts[3:]
	if len(rest) == 1 && rest[0] == "sites" {
		if c.failSiteList[cid] {
			writeError(w, http.StatusOK, CodeGeneralError, "General error.")
			return
		}
		rows := make([]any, 0, len(customer.Sites))
		for _, site := range customer.Sites {
			rows = append(rows, map[string]any{"siteId": site.ID, "name": site.Name})
		}
		c.writePage(w, r, rows)
		return
	}
	if len(rest) >= 2 && rest[0] == "sites" && !hasSite(customer, rest[1]) {
		writeError(w, http.StatusOK, CodeOperationForbidden, "Operation forbidden.")
		return
	}
	// Every other site endpoint: an empty grid is a valid answer.
	c.writePage(w, r, []any{})
}

func (c *Controller) serveWeb(w http.ResponseWriter, r *http.Request, cid, endpoint string, body []byte, webAuthed bool) {
	if endpoint == "login" && r.Method == http.MethodPost {
		var request struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = json.Unmarshal(body, &request)
		if cid != c.MSPID {
			writeError(w, http.StatusOK, CodeGeneralError, "General error.")
			return
		}
		if request.Username != c.Username || request.Password != c.Password {
			writeError(w, http.StatusOK, -30109, "Invalid username or password.")
			return
		}
		session, csrf := c.nextID("session"), c.nextID("csrf")
		c.sessions[session] = csrf
		c.webLogins++
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session, Path: "/", HttpOnly: true})
		writeOK(w, map[string]any{"roleType": 0, "token": csrf})
		return
	}
	if endpoint == "loginStatus" {
		if cid != c.MSPID {
			writeError(w, http.StatusOK, CodeGeneralError, "General error.")
			return
		}
		writeOK(w, map[string]any{"login": webAuthed})
		return
	}
	if !webAuthed {
		writeError(w, http.StatusOK, CodeLoginRequired, "Login required.")
		return
	}
	if cid == c.MSPID {
		switch endpoint {
		case "settings/system/status":
			writeOK(w, map[string]any{"name": c.MSPName, "model": "", "category": ""})
		default:
			// Includes users/current, which is where the old exporter stopped.
			writeError(w, http.StatusOK, CodeGeneralError, "General error.")
		}
		return
	}
	customer, ok := c.customer(cid)
	if !ok || c.forbidden[cid] {
		writeError(w, http.StatusOK, CodeOperationForbidden, "Operation forbidden.")
		return
	}
	switch {
	case endpoint == "users/current":
		sites := make([]any, 0, len(customer.Sites))
		for _, site := range customer.Sites {
			sites = append(sites, map[string]any{"name": site.Name, "key": site.ID})
		}
		writeOK(w, map[string]any{"privilege": map[string]any{"sites": sites}})
	case endpoint == "settings/system/status":
		writeOK(w, map[string]any{"name": customer.Name, "upTime": 3600000, "model": "", "category": ""})
	case endpoint == "sites/alert-count" && r.Method == http.MethodPost:
		var request struct {
			SiteIDs []string `json:"siteIds"`
		}
		_ = json.Unmarshal(body, &request)
		if len(request.SiteIDs) != 1 || !hasSite(customer, request.SiteIDs[0]) {
			writeError(w, http.StatusOK, CodeOperationForbidden, "Operation forbidden.")
			return
		}
		writeOK(w, []any{map[string]any{"alertNum": 2, "obscured": false}})
	case strings.HasPrefix(endpoint, "sites/"):
		siteID := strings.Split(endpoint, "/")[1]
		if !hasSite(customer, siteID) {
			writeError(w, http.StatusOK, CodeOperationForbidden, "Operation forbidden.")
			return
		}
		if strings.HasSuffix(endpoint, "/devices") {
			writeOK(w, []any{})
			return
		}
		writeOK(w, map[string]any{})
	default:
		writeOK(w, map[string]any{})
	}
}

func (c *Controller) writePage(w http.ResponseWriter, r *http.Request, rows []any) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if size < 1 || (c.MaxPageSize > 0 && size > c.MaxPageSize) {
		size = c.MaxPageSize
	}
	if size < 1 {
		size = len(rows)
	}
	start := min((page-1)*size, len(rows))
	end := min(start+size, len(rows))
	writeOK(w, map[string]any{"totalRows": len(rows), "currentPage": page, "currentSize": end - start, "data": rows[start:end]})
}

func hasSite(customer Customer, siteID string) bool {
	for _, site := range customer.Sites {
		if site.ID == siteID {
			return true
		}
	}
	return false
}

func writeOK(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "msg": "Success.", "result": result})
}

func writeError(w http.ResponseWriter, status, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": code, "msg": message})
}
