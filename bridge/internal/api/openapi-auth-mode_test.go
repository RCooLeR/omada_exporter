package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestMissingOpenAPICredentialsLogOneStartupNotice(t *testing.T) {
	for _, level := range []zerolog.Level{
		zerolog.DebugLevel,
		zerolog.InfoLevel,
		zerolog.WarnLevel,
		zerolog.ErrorLevel,
		zerolog.Disabled,
	} {
		t.Run(level.String(), func(t *testing.T) {
			output := captureAuthModeLogs(t, level)
			client := &Client{
				Config:         &config.Config{OpenAPIAuth: config.OpenAPIAuthAuto},
				ControllerKind: config.SystemTypeStandard,
				httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					t.Fatal("disabled OpenAPI must not send an HTTP request")
					return nil, nil
				})},
			}

			if err := client.configureOpenAPIAuth(); err != nil {
				t.Fatalf("configureOpenAPIAuth() error = %v", err)
			}
			if client.OpenAPIAuthMode != config.OpenAPIAuthDisabled {
				t.Fatalf("OpenAPI auth mode = %q, want disabled", client.OpenAPIAuthMode)
			}
			startupOutput := output.String()
			if level == zerolog.Disabled {
				if startupOutput != "" {
					t.Fatalf("disabled logging emitted %q", startupOutput)
				}
			} else {
				lines := strings.Split(strings.TrimSpace(startupOutput), "\n")
				if len(lines) != 1 {
					t.Fatalf("startup emitted %d lines, want one: %q", len(lines), startupOutput)
				}
				var event map[string]any
				if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
					t.Fatalf("invalid startup JSON: %v", err)
				}
				if event["event"] != "openapi_disabled" {
					t.Fatalf("startup event = %v, want openapi_disabled", event["event"])
				}
				if _, exists := event["level"]; exists {
					t.Fatalf("expected configuration notice without severity, got %v", event["level"])
				}
				for _, expected := range []string{"OMADA_CLIENT_ID", "OMADA_SECRET_ID", "client", "ISP", "VPN", "WAN", "network-quality", "Web API collection continues"} {
					if !strings.Contains(startupOutput, expected) {
						t.Errorf("startup notice does not mention %q", expected)
					}
				}
			}

			for range 3 {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://omada.example/openapi/v1/controller/sites/site/clients", nil)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := client.MakeOpenApiRequest(req)
				if err == nil || resp != nil {
					t.Fatalf("disabled OpenAPI request = (%v, %v), want nil response and an error", resp, err)
				}
			}
			if output.String() != startupOutput {
				t.Fatalf("disabled requests emitted additional logs: %q", output.String())
			}
			if zerolog.GlobalLevel() != level {
				t.Fatalf("global logging threshold changed to %v, want %v", zerolog.GlobalLevel(), level)
			}
		})
	}
}

func TestExplicitlyDisabledOpenAPIHasNoMissingCredentialsNotice(t *testing.T) {
	output := captureAuthModeLogs(t, zerolog.DebugLevel)
	client := &Client{Config: &config.Config{OpenAPIAuth: config.OpenAPIAuthDisabled}}
	if err := client.configureOpenAPIAuth(); err != nil {
		t.Fatalf("configureOpenAPIAuth() error = %v", err)
	}
	if client.OpenAPIAuthMode != config.OpenAPIAuthDisabled {
		t.Fatalf("OpenAPI auth mode = %q, want disabled", client.OpenAPIAuthMode)
	}
	if output.Len() != 0 {
		t.Fatalf("explicitly disabled OpenAPI emitted a missing-credentials notice: %q", output.String())
	}
}

func TestIncompleteOpenAPICredentialsFailWithoutDisabledNotice(t *testing.T) {
	for _, mode := range []string{config.OpenAPIAuthAuto, config.OpenAPIAuthClientCredentials} {
		for _, credential := range []string{"client-id", "secret-id"} {
			t.Run(mode+"/"+credential, func(t *testing.T) {
				output := captureAuthModeLogs(t, zerolog.DebugLevel)
				cfg := &config.Config{OpenAPIAuth: mode}
				if credential == "client-id" {
					cfg.ClientId = "synthetic-client-id"
				} else {
					cfg.SecretId = "synthetic-secret-id"
				}
				client := &Client{Config: cfg, ControllerKind: config.SystemTypeFusion}
				if err := client.configureOpenAPIAuth(); err == nil {
					t.Fatal("incomplete OpenAPI credentials must fail startup")
				}
				if client.OpenAPIAuthMode != config.OpenAPIAuthClientCredentials {
					t.Fatalf("OpenAPI auth mode = %q, want client_credentials", client.OpenAPIAuthMode)
				}
				if output.Len() != 0 {
					t.Fatalf("incomplete credentials were silently disabled: %q", output.String())
				}
			})
		}
	}
}

func TestFusionWithoutOpenAPICredentialsUsesWebSession(t *testing.T) {
	output := captureAuthModeLogs(t, zerolog.DebugLevel)
	loginChecks, openAPIRequests := 0, 0
	client := &Client{
		Config:         &config.Config{Host: "https://omada.example", OpenAPIAuth: config.OpenAPIAuthAuto},
		ControllerKind: config.SystemTypeFusion,
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/controller-id/api/v2/loginStatus":
				loginChecks++
				return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
			case "/openapi/v1/controller-id/sites/site-id/clients":
				openAPIRequests++
				if req.Header.Get("Omada-Request-Source") != "web-local" || req.Header.Get("Csrf-Token") != "synthetic-web-token" {
					t.Fatal("Fusion OpenAPI request did not use the web session")
				}
				if req.Header.Get("Authorization") != "" {
					t.Fatal("Fusion web-session request used client-credentials authorization")
				}
				return jsonResponse(`{"errorCode":0,"result":[]}`), nil
			default:
				t.Fatalf("unexpected request path %q", req.URL.Path)
				return nil, nil
			}
		})},
	}
	client.SetContextIDs("controller-id", "site-id")
	client.setWebToken("synthetic-web-token")
	if err := client.configureOpenAPIAuth(); err != nil {
		t.Fatalf("configureOpenAPIAuth() error = %v", err)
	}
	if client.OpenAPIAuthMode != config.OpenAPIAuthWebSession {
		t.Fatalf("OpenAPI auth mode = %q, want web_session", client.OpenAPIAuthMode)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://omada.example/openapi/v1/controller-id/sites/site-id/clients", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.MakeOpenApiRequest(req)
	if err != nil {
		t.Fatalf("MakeOpenApiRequest() error = %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response: %v", err)
	}
	if loginChecks != 2 || openAPIRequests != 1 {
		t.Fatalf("request counts = %d login checks and %d OpenAPI requests, want 2 and 1", loginChecks, openAPIRequests)
	}
	if strings.Contains(output.String(), "openapi_disabled") {
		t.Fatalf("Fusion web-session mode emitted a disabled notice: %q", output.String())
	}
}

func captureAuthModeLogs(t *testing.T, level zerolog.Level) *bytes.Buffer {
	t.Helper()
	previousLogger := log.Logger
	previousLevel := zerolog.GlobalLevel()
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)
	zerolog.SetGlobalLevel(level)
	t.Cleanup(func() {
		log.Logger = previousLogger
		zerolog.SetGlobalLevel(previousLevel)
	})
	return &output
}
