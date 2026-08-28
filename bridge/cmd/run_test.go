package cmd

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestRunExporterRequiresConnectionFlags(t *testing.T) {
	previous := conf
	conf = config.Config{}
	t.Cleanup(func() { conf = previous })

	err := runExporter(context.Background(), nil)
	if err == nil {
		t.Fatal("runExporter() returned nil error without required connection flags")
	}
	for _, flag := range []string{"host", "username", "password"} {
		if !strings.Contains(err.Error(), flag) {
			t.Fatalf("runExporter() error = %q, want missing %s flag", err, flag)
		}
	}
}

func TestNewExporterHTTPServerAppliesLimits(t *testing.T) {
	handler := http.NewServeMux()
	server := newExporterHTTPServer(":9202", handler)

	if server.Addr != ":9202" {
		t.Fatalf("Addr = %q, want :9202", server.Addr)
	}
	if server.Handler != handler {
		t.Fatal("Handler does not match the configured handler")
	}
	if server.ReadHeaderTimeout != exporterReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", server.ReadHeaderTimeout, exporterReadHeaderTimeout)
	}
	if server.ReadTimeout != exporterReadTimeout {
		t.Fatalf("ReadTimeout = %v, want %v", server.ReadTimeout, exporterReadTimeout)
	}
	if server.IdleTimeout != exporterIdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", server.IdleTimeout, exporterIdleTimeout)
	}
	if server.MaxHeaderBytes != exporterMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, exporterMaxHeaderBytes)
	}
	if server.MaxHeaderValueCount != exporterMaxHeaderValueCount {
		t.Fatalf("MaxHeaderValueCount = %d, want %d", server.MaxHeaderValueCount, exporterMaxHeaderValueCount)
	}
}
