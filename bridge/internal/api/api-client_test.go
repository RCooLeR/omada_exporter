package api

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURLString(t *testing.T) {
	got := redactURLString("https://omada.example/openapi/authorize/token?client_id=id&client_secret=secret&refresh_token=refresh&grant_type=refresh_token")

	if got == "" {
		t.Fatal("redactURLString() returned empty URL")
	}
	for _, leaked := range []string{"client_secret=secret", "refresh_token=refresh"} {
		if contains(got, leaked) {
			t.Fatalf("redactURLString() = %q still contains %q", got, leaked)
		}
	}
	for _, expected := range []string{"client_id=id", "client_secret=%3Credacted%3E", "refresh_token=%3Credacted%3E"} {
		if !contains(got, expected) {
			t.Fatalf("redactURLString() = %q missing %q", got, expected)
		}
	}
}

func TestRedactErrorRedactsURLError(t *testing.T) {
	err := &url.Error{
		Op:  "Post",
		URL: "https://omada.example/openapi?token=abc123",
		Err: errors.New("connection refused"),
	}

	got := redactError(err).Error()
	if contains(got, "abc123") {
		t.Fatalf("redactError() = %q still contains token", got)
	}
	if !contains(got, "token=%3Credacted%3E") {
		t.Fatalf("redactError() = %q missing redacted token", got)
	}
}

func contains(value, part string) bool {
	return strings.Contains(value, part)
}
