package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/urfave/cli/v3"
)

func TestCLIResponseDumpFlags(t *testing.T) {
	for _, source := range []string{"default", "flags", "environment"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("OMADA_DUMP_RESPONSES_DIR", "")
			t.Setenv("OMADA_DUMP_RESPONSES_ONLY", "false")
			conf := &config.Config{}
			command := newCLICommand(conf)
			command.Writer = io.Discard
			command.ErrWriter = io.Discard
			command.Action = func(context.Context, *cli.Command) error { return nil }
			args := []string{"omada_exporter"}
			dir := t.TempDir()
			if source == "flags" {
				args = append(args, "--dump-responses-dir", dir, "--dump-responses-only")
			}
			if source == "environment" {
				t.Setenv("OMADA_DUMP_RESPONSES_DIR", dir)
				t.Setenv("OMADA_DUMP_RESPONSES_ONLY", "true")
			}
			if err := command.Run(context.Background(), args); err != nil {
				t.Fatal(err)
			}
			if source == "default" {
				if conf.DumpResponsesDir != "" || conf.DumpResponsesOnly {
					t.Fatal("dump mode must be disabled by default")
				}
			} else if conf.DumpResponsesDir != dir || !conf.DumpResponsesOnly {
				t.Fatalf("dump flags not parsed: dir=%q only=%v", conf.DumpResponsesDir, conf.DumpResponsesOnly)
			}
		})
	}
}

func TestRunExporterValidatesDumpFlagsBeforeConnecting(t *testing.T) {
	for _, conf := range []*config.Config{
		{DumpResponsesOnly: true},
		{DumpResponsesOnly: true, DumpResponsesDir: "  "},
		{DumpResponsesDir: "  "},
	} {
		err := runExporterWithConfig(context.Background(), nil, conf)
		if err == nil || !strings.Contains(err.Error(), "dump-responses-") {
			t.Fatalf("dump flag validation should precede missing credentials or API access: %v", err)
		}
	}
}

func TestRunExporterDumpOnlyExitsWithoutServingOrPublishing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/info":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"omadacId":"cid"}}`)
		case "/cid/api/v2/loginStatus":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"login":true}}`)
		case "/cid/api/v2/users/current":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`)
		case "/cid/api/v2/sites/site-id/devices":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":[]}`)
		case "/cid/api/v2/settings/system/status", "/cid/api/v2/maintenance/software/channelUpdate", "/cid/api/v2/sites/alert-count":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{}}`)
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	err := runExporterWithConfig(context.Background(), nil, &config.Config{
		Host: server.URL, Username: "test-user", Password: "test-pass", Site: "Default",
		SystemType: config.SystemTypeStandard, OpenAPIAuth: config.OpenAPIAuthDisabled,
		Timeout: 1, LogLevel: "error", DumpResponsesDir: dir, DumpResponsesOnly: true,
		Port: "invalid-if-started", MQTTEnabled: true, MQTTBroker: "invalid-if-started",
	})
	if err != nil {
		t.Fatalf("dump-only mode must exit before HTTP/MQTT startup: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("missing dump directory: %v %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(dir, entries[0].Name(), "manifest.json")); err != nil {
		t.Fatalf("missing dump manifest: %v", err)
	}
}
