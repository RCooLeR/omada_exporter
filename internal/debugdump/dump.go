package debugdump

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/rs/zerolog/log"
)

type endpointSpec struct {
	Name       string
	Method     string
	URL        string
	Body       any
	UseOpenAPI bool
}

type deviceRef struct {
	Type       string `json:"type"`
	Mac        string `json:"mac"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	ShowModel  string `json:"showModel"`
	DeviceMisc struct {
		LanPortsNum int `json:"lanPortsNum"`
	} `json:"deviceMisc"`
}

type devicesResponse struct {
	Result []deviceRef `json:"result"`
}

type dumpFile struct {
	Name         string `json:"name"`
	Source       string `json:"source"`
	Method       string `json:"method"`
	URL          string `json:"url"`
	StatusCode   int    `json:"statusCode,omitempty"`
	RetrievedAt  string `json:"retrievedAt"`
	RequestBody  any    `json:"requestBody,omitempty"`
	ResponseBody any    `json:"responseBody,omitempty"`
	ResponseText string `json:"responseText,omitempty"`
	Error        string `json:"error,omitempty"`
}

type manifestFile struct {
	GeneratedAt string   `json:"generatedAt"`
	Host        string   `json:"host"`
	OmadaCID    string   `json:"omadaCid"`
	Site        string   `json:"site"`
	SiteID      string   `json:"siteId"`
	HealthStart int64    `json:"healthStart"`
	HealthEnd   int64    `json:"healthEnd"`
	Files       []string `json:"files"`
}

func DumpResponses(client *api.Client, dir string) error {
	if client == nil {
		return fmt.Errorf("nil client")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	now := time.Now().UTC()
	healthEnd := now.UnixMilli()
	healthStart := now.Add(-24 * time.Hour).UnixMilli()
	files := make([]string, 0, 64)

	webDevicesSpec := endpointSpec{
		Name:   "webapi_site_devices",
		Method: http.MethodGet,
		URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/devices", client.Config.Host, client.OmadaCID, client.SiteId),
	}
	webDevicesFile, webDevicesBody, err := dumpEndpoint(client, dir, webDevicesSpec)
	files = append(files, webDevicesFile)
	if err != nil {
		return fmt.Errorf("dump %s: %w", webDevicesSpec.Name, err)
	}

	devices, err := parseDevices(webDevicesBody)
	if err != nil {
		return fmt.Errorf("parse %s: %w", webDevicesSpec.Name, err)
	}

	globalSpecs := []endpointSpec{
		{
			Name:   "webapi_controller_status",
			Method: http.MethodGet,
			URL:    fmt.Sprintf("%s/%s/api/v2/settings/system/status", client.Config.Host, client.OmadaCID),
		},
		{
			Name:   "webapi_controller_channel_update",
			Method: http.MethodGet,
			URL:    fmt.Sprintf("%s/%s/api/v2/maintenance/software/channelUpdate", client.Config.Host, client.OmadaCID),
		},
		{
			Name:   "webapi_site_alert_count",
			Method: http.MethodPost,
			URL:    fmt.Sprintf("%s/%s/api/v2/sites/alert-count", client.Config.Host, client.OmadaCID),
			Body: map[string]any{
				"siteIds": []string{client.SiteId},
			},
		},
		{
			Name:       "openapi_controller_status",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/system/setting/controller-status", client.Config.Host, client.OmadaCID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_devices_upgradeable_stat",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/devices/upgradeable/stat", client.Config.Host, client.OmadaCID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_devices_page",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/devices?page=1&pageSize=1000", client.Config.Host, client.OmadaCID, client.SiteId),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_devices_all",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/devices/all", client.Config.Host, client.OmadaCID, client.SiteId),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_alerts",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/logs/alerts?page=1&pageSize=1000", client.Config.Host, client.OmadaCID, client.SiteId),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_health_timeline",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/health/timeline?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, healthStart, healthEnd),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_switches_health_timeline",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/health/timeline?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, healthStart, healthEnd),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_wifi_health_timeline",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/wifi/health/timeline?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, healthStart, healthEnd),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_gateway_isp_load",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/dashboard/gateway/isp/load", client.Config.Host, client.OmadaCID, client.SiteId),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_clients",
			Method:     http.MethodPost,
			URL:        fmt.Sprintf("%s/openapi/v2/%s/sites/%s/clients", client.Config.Host, client.OmadaCID, client.SiteId),
			UseOpenAPI: true,
			Body: map[string]any{
				"filters": map[string]any{
					"active": true,
				},
				"sorts":                 map[string]any{},
				"hideHealthUnsupported": true,
				"page":                  1,
				"pageSize":              1000,
				"scope":                 1,
			},
		},
		{
			Name:       "openapi_site_vpn",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/vpn", client.Config.Host, client.OmadaCID, client.SiteId),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_vpn_stats",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/tunnel?page=1&pageSize=1000", client.Config.Host, client.OmadaCID, client.SiteId),
			UseOpenAPI: true,
		},
	}

	for _, spec := range globalSpecs {
		fileName, _, dumpErr := dumpEndpoint(client, dir, spec)
		files = append(files, fileName)
		if dumpErr != nil {
			log.Warn().Err(dumpErr).Str("name", spec.Name).Msg("response dump failed")
		}
	}

	for _, device := range devices {
		deviceSpecs := buildDeviceSpecs(client, device, healthStart, healthEnd)
		for _, spec := range deviceSpecs {
			fileName, _, dumpErr := dumpEndpoint(client, dir, spec)
			files = append(files, fileName)
			if dumpErr != nil {
				log.Warn().Err(dumpErr).Str("name", spec.Name).Str("mac", device.Mac).Msg("response dump failed")
			}
		}
	}

	sort.Strings(files)
	manifest := manifestFile{
		GeneratedAt: now.Format(time.RFC3339),
		Host:        client.Config.Host,
		OmadaCID:    client.OmadaCID,
		Site:        client.Config.Site,
		SiteID:      client.SiteId,
		HealthStart: healthStart,
		HealthEnd:   healthEnd,
		Files:       files,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifestBytes, 0o644); err != nil {
		return err
	}

	log.Info().Str("dir", dir).Int("files", len(files)).Msg("wrote Omada response dump")
	return nil
}

func buildDeviceSpecs(client *api.Client, device deviceRef, healthStart, healthEnd int64) []endpointSpec {
	macSlug := sanitizeSlug(device.Mac)
	baseName := fmt.Sprintf("%s_%s", sanitizeSlug(device.Type), macSlug)
	specs := []endpointSpec{
		{
			Name:       fmt.Sprintf("openapi_%s_latest_firmware", baseName),
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/devices/%s/latest-firmware-info", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
			UseOpenAPI: true,
		},
	}

	switch strings.ToLower(device.Type) {
	case "switch":
		specs = append(specs,
			endpointSpec{
				Name:   fmt.Sprintf("webapi_%s_detail", baseName),
				Method: http.MethodGet,
				URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/switches/%s", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_overview", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/%s", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_stats", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/stat/switches/%s", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_detail", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/%s/health/detail?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_timeline", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/%s/health/timeline?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
		)
	case "ap":
		specs = append(specs,
			endpointSpec{
				Name:   fmt.Sprintf("webapi_%s_ports", baseName),
				Method: http.MethodGet,
				URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/eaps/%s/ports", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_info", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_ports", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/ports", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_radios", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/radios", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_wired_uplink", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/wired-uplink", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_lan_traffic", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/lan-traffic-info", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_wlan_group", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/wlan-group", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_detail", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/eaps/%s/health/detail?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_timeline", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/eaps/%s/health/timeline?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
		)
	case "gateway":
		specs = append(specs,
			endpointSpec{
				Name:   fmt.Sprintf("webapi_%s_detail", baseName),
				Method: http.MethodGet,
				URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/gateways/%s", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_info", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_ports", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/ports", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_wan_status", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/wan-status", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_detail", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/health/detail?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_timeline", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/health/timeline?start=%d&end=%d", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_wan_details", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/health/gateways/%s/wans/details", client.Config.Host, client.OmadaCID, client.SiteId, device.Mac),
				UseOpenAPI: true,
			},
		)
	}

	return specs
}

func dumpEndpoint(client *api.Client, dir string, spec endpointSpec) (string, []byte, error) {
	fileName := sanitizeSlug(spec.Name) + ".json"
	var requestBody []byte
	var err error
	if spec.Body != nil {
		requestBody, err = json.Marshal(spec.Body)
		if err != nil {
			return fileName, nil, err
		}
	}

	var bodyReader io.Reader
	if len(requestBody) > 0 {
		bodyReader = bytes.NewReader(requestBody)
	}
	req, err := http.NewRequest(spec.Method, spec.URL, bodyReader)
	if err != nil {
		return fileName, nil, err
	}
	if len(requestBody) > 0 {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}

	result := dumpFile{
		Name:        spec.Name,
		Source:      sourceName(spec.UseOpenAPI),
		Method:      spec.Method,
		URL:         spec.URL,
		RetrievedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if len(requestBody) > 0 {
		result.RequestBody = json.RawMessage(requestBody)
	}

	resp, requestErr := doRequest(client, req, spec.UseOpenAPI)
	if requestErr != nil {
		result.Error = requestErr.Error()
		if err := writeDumpFile(dir, fileName, result); err != nil {
			return fileName, nil, err
		}
		return fileName, nil, requestErr
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = err.Error()
		if writeErr := writeDumpFile(dir, fileName, result); writeErr != nil {
			return fileName, nil, writeErr
		}
		return fileName, nil, err
	}

	var parsed any
	if len(responseBody) > 0 && json.Unmarshal(responseBody, &parsed) == nil {
		result.ResponseBody = parsed
	} else {
		result.ResponseText = string(responseBody)
	}

	if err := writeDumpFile(dir, fileName, result); err != nil {
		return fileName, nil, err
	}
	return fileName, responseBody, nil
}

func writeDumpFile(dir, fileName string, result dumpFile) error {
	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, fileName), content, 0o644)
}

func doRequest(client *api.Client, req *http.Request, useOpenAPI bool) (*http.Response, error) {
	if useOpenAPI {
		return client.MakeOpenApiRequest(req)
	}
	return client.MakeLoggedInRequest(req)
}

func parseDevices(body []byte) ([]deviceRef, error) {
	var parsed devicesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Result, nil
}

func sourceName(useOpenAPI bool) string {
	if useOpenAPI {
		return "openapi"
	}
	return "webapi"
}

func sanitizeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		":", "_",
		"/", "_",
		"\\", "_",
		" ", "_",
		"-", "_",
		".", "_",
		"?", "_",
		"&", "_",
		"=", "_",
	)
	value = replacer.Replace(value)
	value = strings.Trim(value, "_")
	if value == "" {
		return "response"
	}
	return value
}
