// Package app wires controller data, Prometheus metrics, and the MQTT publisher
// into an exporter runtime independently of CLI parsing.
package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/debugdump"
	"github.com/RCooLeR/omada_exporter/internal/hamqtt"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Run authenticates to the controller and serves metrics until ctx is canceled.
// MQTT publishing is optional; its lifecycle is owned by this invocation.
func Run(ctx context.Context, conf *config.Config) error {
	if err := validateConfig(conf); err != nil {
		return err
	}
	level, err := zerolog.ParseLevel(conf.LogLevel)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(level)
	conf.Host = strings.TrimRight(conf.Host, "/")

	shared, err := session.Configure(conf)
	if err != nil {
		return err
	}
	if conf.DumpResponsesDir != "" {
		if err := debugdump.DumpResponses(ctx, shared, conf.DumpResponsesDir); err != nil {
			return err
		}
		if conf.DumpResponsesOnly {
			return nil
		}
	}
	client := omada.NewClient(shared)
	collectors := initCollectors(client)
	handler, health, err := newExporterHandler(conf, collectors)
	if err != nil {
		return err
	}

	var publish func(context.Context) error
	if conf.MQTTEnabled {
		publisher, err := hamqtt.NewPublisher(client, collectors)
		if err != nil {
			log.Error().Err(err).Msg("home assistant mqtt publisher disabled")
		} else {
			publish = publisher.Run
		}
	}

	log.Info().Msg(fmt.Sprintf("listening on :%s", conf.Port))
	server := newExporterHTTPServer(fmt.Sprintf(":%s", conf.Port), handler)
	return serve(ctx, server, health, publish)
}

func validateConfig(conf *config.Config) error {
	if conf == nil {
		return fmt.Errorf("exporter configuration is nil")
	}
	if err := config.ValidateBridgeID(conf.BridgeID); err != nil {
		return err
	}
	if conf.AllowDeviceControl && !conf.MQTTEnabled {
		return fmt.Errorf("allow-device-control requires mqtt-enabled")
	}
	if conf.DumpResponsesOnly && strings.TrimSpace(conf.DumpResponsesDir) == "" {
		return fmt.Errorf("dump-responses-only requires dump-responses-dir")
	}
	if conf.DumpResponsesDir != "" && strings.TrimSpace(conf.DumpResponsesDir) == "" {
		return fmt.Errorf("dump-responses-dir must not be blank")
	}
	missing := make([]string, 0, 3)
	if conf.Host == "" {
		missing = append(missing, "host")
	}
	if conf.Username == "" {
		missing = append(missing, "username")
	}
	if conf.Password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required flags %q not set", strings.Join(missing, ", "))
	}

	return nil
}
