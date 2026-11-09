package omada

import (
	"errors"
	"testing"
)

func TestIsUnsupportedClientsV2Error(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"omada -1600", errors.New("clients returned HTTP 200 OK: errorCode -1600: Unsupported request path."), true},
		{"unsupported path message", errors.New("clients: Unsupported request path"), true},
		// Omada Software Controller 5.15.24 has no /openapi/v2/.../clients and
		// answers with a plain Spring 404 body, so readResponseBody yields no errorCode.
		{"plain 404 from 5.15 controller", errors.New("clients returned HTTP 404 "), true},
		{"404 with status text", errors.New("clients returned HTTP 404 Not Found"), true},
		{"OpenAPI login 404", errors.New("OpenAPI login returned HTTP 404 Not Found"), false},
		{"web login 404", errors.New("web login returned HTTP 404 Not Found"), false},
		{"login status 404", errors.New("login status returned HTTP 404 Not Found"), false},
		{"controller info 404", errors.New("controller info returned HTTP 404 Not Found"), false},
		{"auth failure", errors.New("clients returned HTTP 401 Unauthorized"), false},
		{"server error", errors.New("clients returned HTTP 500 Internal Server Error"), false},
		{"other api error", errors.New("clients returned HTTP 200 OK: errorCode -44112: token expired"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnsupportedClientsV2Error(tt.err); got != tt.want {
				t.Fatalf("isUnsupportedClientsV2Error(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
