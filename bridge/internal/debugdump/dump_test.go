package debugdump

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

type fakeClient struct {
	handler func(*http.Request, bool) (*http.Response, error)
}

func (f *fakeClient) ContextIDs() (string, string) { return "cid", "site-id" }

func (f *fakeClient) MakeLoggedInRequest(req *http.Request) (*http.Response, error) {
	return f.handler(req, false)
}

func (f *fakeClient) MakeOpenApiRequest(req *http.Request) (*http.Response, error) {
	return f.handler(req, true)
}

func response(code int, body string) *http.Response {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(code)
	_, _ = recorder.WriteString(body)
	return recorder.Result()
}

func readJSON[T any](t *testing.T, path string) T {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func onlyRunDir(t *testing.T, parent string) string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("run directories = %v, want one directory", entries)
	}
	return filepath.Join(parent, entries[0].Name())
}

func TestDumpResponsesPreservesReadOnlyQueriesAndManifest(t *testing.T) {
	var paths []string
	posts := 0
	client := &fakeClient{handler: func(req *http.Request, openAPI bool) (*http.Response, error) {
		if strings.HasPrefix(req.URL.Path, "/openapi/") != openAPI {
			t.Errorf("wrong auth wrapper for %s", req.URL.Path)
		}
		paths = append(paths, req.URL.Path)
		switch req.Method {
		case http.MethodGet:
		case http.MethodPost:
			posts++
			if req.URL.Path != "/cid/api/v2/sites/alert-count" && req.URL.Path != "/openapi/v2/cid/sites/site-id/clients" {
				t.Fatalf("unexpected POST path %s", req.URL.Path)
			}
			if req.GetBody == nil || req.Header.Get("Content-Type") != "application/json;charset=UTF-8" {
				t.Fatal("query body must be replayable JSON")
			}
			body, err := io.ReadAll(req.Body)
			if err != nil || !json.Valid(body) {
				t.Fatalf("invalid request body %s: %v", body, err)
			}
		default:
			t.Fatalf("unexpected write method %s", req.Method)
		}
		if req.URL.Path == "/cid/api/v2/sites/site-id/devices" {
			return response(200, `{"errorCode":0,"result":[{"type":"switch","mac":"00:11:22:33:44:55"},{"type":"ap","mac":"00:11:22:33:44:66"},{"type":"gateway","mac":"00:11:22:33:44:77"}]}`), nil
		}
		return response(200, `{"errorCode":0,"result":{"largeCounter":9007199254740993}}`), nil
	}}
	parent := t.TempDir()
	conf := &config.Config{Host: "https://controller.invalid", Site: "Default"}
	if err := dumpResponses(context.Background(), client, conf, parent); err != nil {
		t.Fatal(err)
	}
	dir := onlyRunDir(t, parent)
	manifest := readJSON[manifestFile](t, filepath.Join(dir, "manifest.json"))
	if manifest.Host != conf.Host || manifest.SiteID != "site-id" || manifest.OmadaCID != "cid" || manifest.FailedEndpoints != 0 {
		t.Fatalf("incorrect manifest: %+v", manifest)
	}
	if manifest.HealthEnd-manifest.HealthStart != 24*60*60*1000 {
		t.Fatal("unexpected health window")
	}
	// Original branch probes: 16 site/controller endpoints + 6 switch + 10 AP + 8 gateway.
	if len(paths) != 40 || len(manifest.Files) != len(paths) || posts != 2 {
		t.Fatalf("requests=%d files=%d posts=%d, want 40/40/2", len(paths), len(manifest.Files), posts)
	}
	if !sort.StringsAreSorted(manifest.Files) {
		t.Fatal("manifest is not sorted")
	}
	for _, name := range manifest.Files {
		if filepath.Base(name) != name {
			t.Fatalf("unsafe filename %s", name)
		}
		result := readJSON[dumpFile](t, filepath.Join(dir, name))
		if result.Error != "" || result.StatusCode != 200 {
			t.Fatalf("unexpected result %+v", result)
		}
	}
	content, err := os.ReadFile(filepath.Join(dir, "webapi_controller_status.json"))
	if err != nil || !strings.Contains(string(content), "9007199254740993") {
		t.Fatalf("large JSON integer lost precision: %s %v", content, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("directory is not private: %v %v", info, err)
		}
		for _, name := range append(manifest.Files, "manifest.json") {
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("file %s is not private: %v %v", name, info, err)
			}
		}
	}
	if err := dumpResponses(context.Background(), client, conf, parent); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 2 {
		t.Fatalf("repeat dump reused old directory: %v %v", entries, err)
	}
}

func TestDumpResponsesRecordsOptionalErrorsAndContinues(t *testing.T) {
	client := &fakeClient{handler: func(req *http.Request, _ bool) (*http.Response, error) {
		switch req.URL.Path {
		case "/cid/api/v2/sites/site-id/devices":
			return response(200, `{"errorCode":0,"result":[]}`), nil
		case "/cid/api/v2/settings/system/status":
			return response(503, `{"errorCode":-1,"msg":"offline"}`), nil
		case "/cid/api/v2/maintenance/software/channelUpdate":
			return response(200, `{"errorCode":-1600,"msg":"unsupported"}`), nil
		default:
			return response(200, `{"errorCode":0,"result":{}}`), nil
		}
	}}
	parent := t.TempDir()
	if err := dumpResponses(context.Background(), client, &config.Config{Host: "https://controller.invalid"}, parent); err != nil {
		t.Fatal(err)
	}
	dir := onlyRunDir(t, parent)
	manifest := readJSON[manifestFile](t, filepath.Join(dir, "manifest.json"))
	if len(manifest.Files) != 16 || manifest.FailedEndpoints != 2 {
		t.Fatalf("incorrect failure manifest: %+v", manifest)
	}
	for _, name := range []string{"webapi_controller_status", "webapi_controller_channel_update"} {
		result := readJSON[dumpFile](t, filepath.Join(dir, name+".json"))
		if result.Error == "" || result.ResponseBody == nil {
			t.Fatalf("error and response were not preserved: %+v", result)
		}
	}
}

func TestDumpResponsesRejectsInvalidDeviceInventory(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `{"result":null}`, `{"result":{}}`, `{"errorCode":-1,"result":[]}`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			client := &fakeClient{handler: func(_ *http.Request, _ bool) (*http.Response, error) {
				calls++
				return response(200, body), nil
			}}
			parent := t.TempDir()
			if err := dumpResponses(context.Background(), client, &config.Config{Host: "https://controller.invalid"}, parent); err == nil {
				t.Fatal("invalid inventory accepted")
			}
			if calls != 1 {
				t.Fatalf("queried %d endpoints after invalid device inventory", calls)
			}
			manifest := readJSON[manifestFile](t, filepath.Join(onlyRunDir(t, parent), "manifest.json"))
			if manifest.FailedEndpoints != 1 {
				t.Fatalf("invalid inventory missing from failures: %+v", manifest)
			}
		})
	}
}

func TestDumpEndpointRecordsTransportError(t *testing.T) {
	want := errors.New("connection failed")
	client := &fakeClient{handler: func(_ *http.Request, _ bool) (*http.Response, error) { return nil, want }}
	dir := t.TempDir()
	_, _, err := dumpEndpoint(context.Background(), client, dir, endpointSpec{Name: "failure", Method: http.MethodGet, URL: "https://controller.invalid"})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
	result := readJSON[dumpFile](t, filepath.Join(dir, "failure.json"))
	if result.Error != want.Error() {
		t.Fatalf("transport error not saved: %+v", result)
	}
}

func TestDumpResponsesCancelsInFlightRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := &fakeClient{handler: func(req *http.Request, _ bool) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(200, `{"errorCode":0,"result":[]}`), nil
		}
		cancel()
		return nil, req.Context().Err()
	}}
	parent := t.TempDir()
	err := dumpResponses(ctx, client, &config.Config{Host: "https://controller.invalid"}, parent)
	if !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatalf("cancellation error=%v calls=%d", err, calls)
	}
	manifest := readJSON[manifestFile](t, filepath.Join(onlyRunDir(t, parent), "manifest.json"))
	if manifest.FailedEndpoints != 1 || len(manifest.Files) != 2 {
		t.Fatalf("canceled dump manifest = %+v", manifest)
	}
	before, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := dumpResponses(ctx, client, &config.Config{}, parent); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled context was not rejected: %v", err)
	}
	after, err := os.ReadDir(parent)
	if err != nil || len(after) != len(before) {
		t.Fatalf("already canceled dump created files: %v", err)
	}
}

type repeatingReader struct{ read int64 }

func (r *repeatingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.read += int64(len(p))
	return len(p), nil
}

func TestDumpEndpointBoundsResponseBody(t *testing.T) {
	body := &repeatingReader{}
	client := &fakeClient{handler: func(_ *http.Request, _ bool) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(body)}, nil
	}}
	dir := t.TempDir()
	_, _, err := dumpEndpoint(context.Background(), client, dir, endpointSpec{Name: "large", Method: http.MethodGet, URL: "https://controller.invalid"})
	if err == nil || !strings.Contains(err.Error(), "exceeds") || body.read != maxDumpResponseBytes+1 {
		t.Fatalf("unbounded body: read=%d err=%v", body.read, err)
	}
	result := readJSON[dumpFile](t, filepath.Join(dir, "large.json"))
	if result.Error == "" || result.ResponseBody != nil || result.ResponseText != "" {
		t.Fatal("oversized body retained")
	}
}

func TestWriteDumpFileNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := writeDumpFile(dir, "result.json", dumpFile{Name: "original"}); err != nil {
		t.Fatal(err)
	}
	err := writeDumpFile(dir, "result.json", dumpFile{Name: "replacement"})
	var outputErr outputError
	if !errors.As(err, &outputErr) {
		t.Fatalf("overwrite did not fail with output error: %v", err)
	}
	if got := readJSON[dumpFile](t, filepath.Join(dir, "result.json")); got.Name != "original" {
		t.Fatalf("original overwritten: %+v", got)
	}
}

func TestBuildDeviceSpecsUsesCurrentContextAndReadOnlyRoutes(t *testing.T) {
	for _, test := range []struct {
		kind  string
		count int
	}{
		{"switch", 6}, {"AP", 10}, {"gateway", 8}, {"unknown", 1},
	} {
		t.Run(test.kind, func(t *testing.T) {
			specs := buildDeviceSpecs(&fakeClient{}, "https://controller.invalid", deviceRef{Type: test.kind, Mac: "00:11:22:33:44:55"}, 123, 456)
			if len(specs) != test.count {
				t.Fatalf("endpoints=%d want=%d", len(specs), test.count)
			}
			for _, spec := range specs {
				if spec.Method != http.MethodGet || spec.Body != nil || !strings.Contains(spec.URL, "/cid/") || !strings.Contains(spec.URL, "/sites/site-id/") {
					t.Fatalf("incorrect route: %+v", spec)
				}
				if strings.Contains(spec.URL, "start=") && !strings.Contains(spec.URL, "start=123&end=456") {
					t.Fatalf("incorrect health window: %s", spec.URL)
				}
			}
		})
	}
}

func TestDumpResponsesRejectsNilClient(t *testing.T) {
	if err := DumpResponses(context.Background(), nil, t.TempDir()); err == nil {
		t.Fatal("nil client accepted")
	}
}
