package cmd

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestCLIFlagValidatorsRejectInvalidConfiguration(t *testing.T) {
	type validatorTest struct {
		name string
		args []string
		want string
	}
	tests := []validatorTest{
		{name: "controller URL", args: []string{"--host", "omada.local"}, want: "absolute http or https URL"},
		{name: "controller URL query", args: []string{"--host", "https://omada.local?tenant=default"}, want: "must not contain a query or fragment"},
		{name: "controller URL fragment", args: []string{"--host", "https://omada.local/#login"}, want: "must not contain a query or fragment"},
		{name: "controller URL port", args: []string{"--host", "https://omada.local:65536"}, want: "invalid host URL port"},
		{name: "port", args: []string{"--port", "70000"}, want: "between 1 and 65535"},
		{name: "system type", args: []string{"--system-type", "appliance"}, want: "system-type must be one of"},
		{name: "OpenAPI auth", args: []string{"--openapi-auth", "magic"}, want: "openapi-auth must be one of"},
		{name: "request timeout", args: []string{"--timeout", "0"}, want: "timeout must be greater than zero"},
		{name: "cache TTL", args: []string{"--cache-ttl", "-1"}, want: "cache-ttl must not be negative"},
		{name: "insight window", args: []string{"--insight-window-seconds", "0"}, want: "insight-window-seconds must be greater than zero"},
		{name: "insight limit", args: []string{"--insight-application-limit", "-1"}, want: "insight-application-limit must not be negative"},
		{name: "MQTT interval", args: []string{"--mqtt-interval", "0"}, want: "mqtt-interval must be greater than zero"},
		{name: "MQTT expiry", args: []string{"--mqtt-expire-after", "-1"}, want: "mqtt-expire-after must not be negative"},
		{name: "log level", args: []string{"--log-level", "verbose"}, want: "invalid log-level"},
	}
	if strconv.IntSize == 64 {
		overflow := strconv.FormatInt(maxSecondFlagValue+1, 10)
		tests = append(tests,
			validatorTest{name: "request timeout overflow", args: []string{"--timeout", overflow}, want: "timeout must not exceed"},
			validatorTest{name: "cache TTL overflow", args: []string{"--cache-ttl", overflow}, want: "cache-ttl must not exceed"},
			validatorTest{name: "insight window overflow", args: []string{"--insight-window-seconds", overflow}, want: "insight-window-seconds must not exceed"},
			validatorTest{name: "MQTT interval overflow", args: []string{"--mqtt-interval", overflow}, want: "mqtt-interval must not exceed"},
			validatorTest{name: "MQTT expiry overflow", args: []string{"--mqtt-expire-after", overflow}, want: "mqtt-expire-after must not exceed"},
		)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newCLICommand(&config.Config{})
			command.Writer = io.Discard
			command.ErrWriter = io.Discard
			err := command.Run(context.Background(), append([]string{"omada_exporter"}, test.args...))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run(%q) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestCLIValidatorBoundaries(t *testing.T) {
	maxSafeSeconds := int(^uint(0) >> 1)
	if strconv.IntSize == 64 {
		limit := maxSecondFlagValue
		maxSafeSeconds = int(limit)
	}
	for name, validate := range map[string]func(int) error{
		"positive seconds":     positiveSeconds("duration"),
		"non-negative seconds": nonNegativeSeconds("duration"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validate(maxSafeSeconds); err != nil {
				t.Fatalf("maximum safe duration rejected: %v", err)
			}
		})
	}
	if err := validateControllerURL("https://omada.local:65535/controller"); err != nil {
		t.Fatalf("maximum valid URL port rejected: %v", err)
	}
	if err := validatePort("65535"); err != nil {
		t.Fatalf("maximum exporter port rejected: %v", err)
	}
}

func TestCLIFlagsCanOnlyBeSpecifiedOnce(t *testing.T) {
	command := newCLICommand(&config.Config{})
	command.Writer = io.Discard
	command.ErrWriter = io.Discard
	err := command.Run(context.Background(), []string{"omada_exporter", "--port", "9202", "--port", "9203"})
	if err == nil || !strings.Contains(err.Error(), "can't duplicate this flag") {
		t.Fatalf("duplicate flag error = %v", err)
	}
}

func TestCLISuggestsMistypedFlags(t *testing.T) {
	var errors bytes.Buffer
	command := newCLICommand(&config.Config{})
	command.Writer = io.Discard
	command.ErrWriter = &errors
	err := command.Run(context.Background(), []string{"omada_exporter", "--prot", "9202"})
	if err == nil {
		t.Fatal("mistyped flag returned nil error")
	}
	if output := errors.String(); !strings.Contains(output, `Did you mean "--port"?`) {
		t.Fatalf("suggestion output = %q, want port suggestion", output)
	}
}

func TestCLIInformationalCommandsDoNotRequireConnectionFlags(t *testing.T) {
	for _, name := range []string{"version", "mdocs"} {
		t.Run(name, func(t *testing.T) {
			command := newCLICommand(&config.Config{})
			command.Writer = io.Discard
			command.ErrWriter = io.Discard
			if err := command.Run(context.Background(), []string{"omada_exporter", name}); err != nil {
				t.Fatalf("%s command error = %v", name, err)
			}
		})
	}
}
