package app

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestMetricDocsAreDeterministicAndComplete(t *testing.T) {
	var first, second bytes.Buffer
	if err := WriteMetricDocs(&first); err != nil {
		t.Fatal(err)
	}
	if err := WriteMetricDocs(&second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("metric documentation depends on collector map order")
	}
	rows := strings.Split(strings.TrimSpace(first.String()), "\n")
	if len(rows) < 3 || rows[0] != "| Name | Description | Labels |" || !slices.IsSorted(rows[2:]) {
		t.Fatal("metric documentation is missing its table header or stable order")
	}
	for _, metric := range []string{"omada_client_connected_total", "omada_dpi_total_traffic_bytes", "omada_gateway_wan_quality_"} {
		if !strings.Contains(first.String(), metric) {
			t.Errorf("offline metric documentation omits %s", metric)
		}
	}
}

type failingDocsWriter struct {
	allowedWrites int
	err           error
}

func (writer *failingDocsWriter) Write(data []byte) (int, error) {
	if writer.allowedWrites == 0 {
		return 0, writer.err
	}
	writer.allowedWrites--
	return len(data), nil
}

func TestMetricDocsPropagateWriteErrors(t *testing.T) {
	wantErr := errors.New("output unavailable")
	for _, allowedWrites := range []int{0, 1, 3} {
		writer := &failingDocsWriter{allowedWrites: allowedWrites, err: wantErr}
		if err := WriteMetricDocs(writer); !errors.Is(err, wantErr) {
			t.Fatalf("error after %d writes = %v, want %v", allowedWrites, err, wantErr)
		}
	}
}
