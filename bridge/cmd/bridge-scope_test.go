package cmd

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestBridgeIDValidation(t *testing.T) {
	for _, id := range []string{"", "a", "slobidska44", "apartment-2", strings.Repeat("a", 64)} {
		if err := config.ValidateBridgeID(id); err != nil {
			t.Errorf("valid bridge ID %q rejected: %v", id, err)
		}
	}
	for _, id := range []string{" ", "Apartment", "apartment_2", "-a", "a-", "a/b", "a+b", "a#b", strings.Repeat("a", 65)} {
		if err := config.ValidateBridgeID(id); err == nil {
			t.Errorf("invalid bridge ID %q accepted", id)
		}
	}
}

func TestBridgeIDCLIAndLegacyDefaults(t *testing.T) {
	for _, id := range []string{"", "slobidska44"} {
		t.Run("id="+id, func(t *testing.T) {
			t.Setenv("OMADA_BRIDGE_ID", id)
			t.Setenv("OMADA_MQTT_CLIENT_ID", "omada_exporter")
			t.Setenv("OMADA_MQTT_TOPIC_PREFIX", "omada_exporter")
			conf := &config.Config{}
			command := newCLICommand(conf)
			command.Writer, command.ErrWriter = io.Discard, io.Discard
			if err := command.Run(context.Background(), []string{"omada_exporter", "version"}); err != nil {
				t.Fatal(err)
			}
			if conf.BridgeID != id || conf.MQTTClientID != "omada_exporter" || conf.MQTTTopicPrefix != "omada_exporter" {
				t.Fatal("bridge ID parsing must not mutate base MQTT configuration")
			}
		})
	}
	command := newCLICommand(&config.Config{})
	command.Writer, command.ErrWriter = io.Discard, io.Discard
	if err := command.Run(context.Background(), []string{"omada_exporter", "--bridge-id", "a/b", "version"}); err == nil {
		t.Fatal("invalid namespace passed CLI validation")
	}
}
