package app

import (
	"context"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestDeviceControlNeedsMQTT(t *testing.T) {
	err := Run(context.Background(), &config.Config{AllowDeviceControl: true})
	if err == nil || !strings.Contains(err.Error(), "requires mqtt-enabled") {
		t.Fatalf("controls without MQTT: %v", err)
	}
}
