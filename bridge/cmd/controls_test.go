package cmd

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestDeviceControlRequiresExplicitOptIn(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"true", true}} {
		t.Run("env="+test.value, func(t *testing.T) {
			t.Setenv("OMADA_ALLOW_DEVICE_CONTROL", test.value)
			conf := &config.Config{}
			command := newCLICommand(conf)
			command.Writer, command.ErrWriter = io.Discard, io.Discard
			if err := command.Run(context.Background(), []string{"omada-exporter", "version"}); err != nil {
				t.Fatal(err)
			}
			if conf.AllowDeviceControl != test.want {
				t.Fatalf("AllowDeviceControl = %v, want %v", conf.AllowDeviceControl, test.want)
			}
		})
	}
}

func TestDeviceControlNeedsMQTT(t *testing.T) {
	err := runExporterWithConfig(context.Background(), nil, &config.Config{AllowDeviceControl: true})
	if err == nil || !strings.Contains(err.Error(), "requires mqtt-enabled") {
		t.Fatalf("controls without MQTT: %v", err)
	}
}
