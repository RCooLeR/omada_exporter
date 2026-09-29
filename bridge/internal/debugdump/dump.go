package debugdump

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
)

const maxDumpResponseBytes int64 = 32 << 20

type requestClient interface {
	ContextIDs() (string, string)
	MakeLoggedInRequest(*http.Request) (*http.Response, error)
	MakeOpenApiRequest(*http.Request) (*http.Response, error)
}

type outputError struct{ error }

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
	GeneratedAt     string   `json:"generatedAt"`
	Host            string   `json:"host"`
	OmadaCID        string   `json:"omadaCid"`
	Site            string   `json:"site"`
	SiteID          string   `json:"siteId"`
	HealthStart     int64    `json:"healthStart"`
	HealthEnd       int64    `json:"healthEnd"`
	Files           []string `json:"files"`
	FailedEndpoints int      `json:"failedEndpoints"`
}

// DumpResponses writes a private diagnostic snapshot to a new directory below dir.
// Raw responses contain private network data and must not be published unreviewed.
func DumpResponses(ctx context.Context, client *api.Client, dir string) error {
	if client == nil || client.Config == nil {
		return fmt.Errorf("nil client or configuration")
	}
	return dumpResponses(ctx, client, client.Config, dir)
}

func dumpResponses(ctx context.Context, client requestClient, conf *config.Config, parentDir string) (returnErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(parentDir) == "" {
		return fmt.Errorf("response dump directory must not be empty")
	}
	if err := os.MkdirAll(parentDir, 0o700); err != nil {
		return err
	}
	now := time.Now().UTC()
	dir, err := os.MkdirTemp(parentDir, "omada-"+now.Format("20060102T150405Z")+"-")
	if err != nil {
		return err
	}
	log.Warn().Str("dir", dir).Msg("writing raw diagnostic responses; these files contain private network data")

	healthEnd := now.UnixMilli()
	healthStart := now.Add(-24 * time.Hour).UnixMilli()
	files := make([]string, 0, 64)
	failedEndpoints := 0
	omadaCID, siteID := client.ContextIDs()
	defer func() {
		sort.Strings(files)
		manifest := manifestFile{
			GeneratedAt:     now.Format(time.RFC3339),
			Host:            conf.Host,
			OmadaCID:        omadaCID,
			Site:            conf.Site,
			SiteID:          siteID,
			HealthStart:     healthStart,
			HealthEnd:       healthEnd,
			Files:           files,
			FailedEndpoints: failedEndpoints,
		}
		if err := writeJSONFile(dir, "manifest.json", manifest); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	webDevicesSpec := endpointSpec{
		Name:   "webapi_site_devices",
		Method: http.MethodGet,
		URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/devices", conf.Host, omadaCID, siteID),
	}
	webDevicesFile, webDevicesBody, err := dumpEndpoint(ctx, client, dir, webDevicesSpec)
	files = append(files, webDevicesFile)
	if err != nil {
		failedEndpoints++
		return fmt.Errorf("dump %s: %w", webDevicesSpec.Name, err)
	}

	devices, err := parseDevices(webDevicesBody)
	if err != nil {
		failedEndpoints++
		return fmt.Errorf("parse %s: %w", webDevicesSpec.Name, err)
	}

	globalSpecs := []endpointSpec{
		{
			Name:   "webapi_controller_status",
			Method: http.MethodGet,
			URL:    fmt.Sprintf("%s/%s/api/v2/settings/system/status", conf.Host, omadaCID),
		},
		{
			Name:   "webapi_controller_channel_update",
			Method: http.MethodGet,
			URL:    fmt.Sprintf("%s/%s/api/v2/maintenance/software/channelUpdate", conf.Host, omadaCID),
		},
		{
			Name:   "webapi_site_alert_count",
			Method: http.MethodPost,
			URL:    fmt.Sprintf("%s/%s/api/v2/sites/alert-count", conf.Host, omadaCID),
			Body: map[string]any{
				"siteIds": []string{siteID},
			},
		},
		{
			Name:       "openapi_controller_status",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/system/setting/controller-status", conf.Host, omadaCID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_devices_upgradeable_stat",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/devices/upgradeable/stat", conf.Host, omadaCID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_devices_page",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/devices?page=1&pageSize=1000", conf.Host, omadaCID, siteID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_devices_all",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/devices/all", conf.Host, omadaCID, siteID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_alerts",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/logs/alerts?page=1&pageSize=1000", conf.Host, omadaCID, siteID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_health_timeline",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/health/timeline?start=%d&end=%d", conf.Host, omadaCID, siteID, healthStart, healthEnd),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_switches_health_timeline",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/health/timeline?start=%d&end=%d", conf.Host, omadaCID, siteID, healthStart, healthEnd),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_wifi_health_timeline",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/wifi/health/timeline?start=%d&end=%d", conf.Host, omadaCID, siteID, healthStart, healthEnd),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_gateway_isp_load",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/dashboard/gateway/isp/load", conf.Host, omadaCID, siteID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_clients",
			Method:     http.MethodPost,
			URL:        fmt.Sprintf("%s/openapi/v2/%s/sites/%s/clients", conf.Host, omadaCID, siteID),
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
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/vpn", conf.Host, omadaCID, siteID),
			UseOpenAPI: true,
		},
		{
			Name:       "openapi_site_vpn_stats",
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/setting/vpn/stats/tunnel?page=1&pageSize=1000", conf.Host, omadaCID, siteID),
			UseOpenAPI: true,
		},
	}

	for _, spec := range globalSpecs {
		fileName, _, dumpErr := dumpEndpoint(ctx, client, dir, spec)
		files = append(files, fileName)
		if dumpErr != nil {
			failedEndpoints++
			var fileErr outputError
			if errors.As(dumpErr, &fileErr) || ctx.Err() != nil {
				return dumpErr
			}
			log.Warn().Err(dumpErr).Str("name", spec.Name).Msg("response dump failed")
		}
	}

	for _, device := range devices {
		deviceSpecs := buildDeviceSpecs(client, conf.Host, device, healthStart, healthEnd)
		for _, spec := range deviceSpecs {
			fileName, _, dumpErr := dumpEndpoint(ctx, client, dir, spec)
			files = append(files, fileName)
			if dumpErr != nil {
				failedEndpoints++
				var fileErr outputError
				if errors.As(dumpErr, &fileErr) || ctx.Err() != nil {
					return dumpErr
				}
				log.Warn().Err(dumpErr).Str("name", spec.Name).Str("mac", device.Mac).Msg("response dump failed")
			}
		}
	}

	log.Info().Str("dir", dir).Int("files", len(files)).Msg("wrote Omada response dump")
	return nil
}

func buildDeviceSpecs(client requestClient, host string, device deviceRef, healthStart, healthEnd int64) []endpointSpec {
	omadaCID, siteID := client.ContextIDs()
	macSlug := sanitizeSlug(device.Mac)
	baseName := fmt.Sprintf("%s_%s", sanitizeSlug(device.Type), macSlug)
	specs := []endpointSpec{
		{
			Name:       fmt.Sprintf("openapi_%s_latest_firmware", baseName),
			Method:     http.MethodGet,
			URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/devices/%s/latest-firmware-info", host, omadaCID, siteID, device.Mac),
			UseOpenAPI: true,
		},
	}

	switch strings.ToLower(device.Type) {
	case "switch":
		specs = append(specs,
			endpointSpec{
				Name:   fmt.Sprintf("webapi_%s_detail", baseName),
				Method: http.MethodGet,
				URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/switches/%s", host, omadaCID, siteID, device.Mac),
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_overview", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/%s", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_stats", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/stat/switches/%s", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_detail", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/%s/health/detail?start=%d&end=%d", host, omadaCID, siteID, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_timeline", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/switches/%s/health/timeline?start=%d&end=%d", host, omadaCID, siteID, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
		)
	case "ap":
		specs = append(specs,
			endpointSpec{
				Name:   fmt.Sprintf("webapi_%s_ports", baseName),
				Method: http.MethodGet,
				URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/eaps/%s/ports", host, omadaCID, siteID, device.Mac),
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_info", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_ports", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/ports", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_radios", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/radios", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_wired_uplink", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/wired-uplink", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_lan_traffic", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/lan-traffic-info", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_wlan_group", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/aps/%s/wlan-group", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_detail", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/eaps/%s/health/detail?start=%d&end=%d", host, omadaCID, siteID, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_timeline", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/eaps/%s/health/timeline?start=%d&end=%d", host, omadaCID, siteID, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
		)
	case "gateway":
		specs = append(specs,
			endpointSpec{
				Name:   fmt.Sprintf("webapi_%s_detail", baseName),
				Method: http.MethodGet,
				URL:    fmt.Sprintf("%s/%s/api/v2/sites/%s/gateways/%s", host, omadaCID, siteID, device.Mac),
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_info", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_ports", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/ports", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_wan_status", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/wan-status", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_detail", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/health/detail?start=%d&end=%d", host, omadaCID, siteID, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_timeline", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/gateways/%s/health/timeline?start=%d&end=%d", host, omadaCID, siteID, device.Mac, healthStart, healthEnd),
				UseOpenAPI: true,
			},
			endpointSpec{
				Name:       fmt.Sprintf("openapi_%s_health_wan_details", baseName),
				Method:     http.MethodGet,
				URL:        fmt.Sprintf("%s/openapi/v1/%s/sites/%s/health/gateways/%s/wans/details", host, omadaCID, siteID, device.Mac),
				UseOpenAPI: true,
			},
		)
	}

	return specs
}

func dumpEndpoint(ctx context.Context, client requestClient, dir string, spec endpointSpec) (string, []byte, error) {
	fileName := sanitizeSlug(spec.Name) + ".json"
	if err := ctx.Err(); err != nil {
		return fileName, nil, err
	}
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
	req, err := http.NewRequestWithContext(ctx, spec.Method, spec.URL, bodyReader)
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
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxDumpResponseBytes+1))
	if err == nil && int64(len(responseBody)) > maxDumpResponseBytes {
		err = fmt.Errorf("response body exceeds %d bytes", maxDumpResponseBytes)
	}
	if err != nil {
		result.Error = err.Error()
		if writeErr := writeDumpFile(dir, fileName, result); writeErr != nil {
			return fileName, nil, writeErr
		}
		return fileName, nil, err
	}

	if json.Valid(responseBody) {
		result.ResponseBody = json.RawMessage(responseBody)
	} else {
		result.ResponseText = string(responseBody)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err = fmt.Errorf("%s returned HTTP %d", spec.Name, resp.StatusCode)
	} else {
		err = api.ValidateAPIResponse(responseBody, spec.Name)
	}
	if err != nil {
		result.Error = err.Error()
	}
	if writeErr := writeDumpFile(dir, fileName, result); writeErr != nil {
		return fileName, nil, writeErr
	}
	return fileName, responseBody, err
}

func writeDumpFile(dir, fileName string, result dumpFile) error {
	return writeJSONFile(dir, fileName, result)
}

func writeJSONFile(dir, fileName string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return outputError{err}
	}
	file, err := os.OpenFile(filepath.Join(dir, fileName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return outputError{err}
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return outputError{err}
	}
	return nil
}

func doRequest(client requestClient, req *http.Request, useOpenAPI bool) (*http.Response, error) {
	if useOpenAPI {
		return client.MakeOpenApiRequest(req)
	}
	return client.MakeLoggedInRequest(req)
}

func parseDevices(body []byte) ([]deviceRef, error) {
	if err := api.ValidateAPIResponse(body, "devices"); err != nil {
		return nil, err
	}
	var parsed devicesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Result == nil {
		return nil, fmt.Errorf("devices response is missing the result array")
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
