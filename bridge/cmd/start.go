package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog"
	log "github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

var version = "dev"

const maxSecondFlagValue int64 = (1<<63 - 1) / int64(time.Second)

// Start configures and runs the CLI application.
func Start() {
	conf := &config.Config{}
	app := newCLICommand(conf)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := app.Run(ctx, os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("App failed to run")
	}
}

func newCLICommand(conf *config.Config) *cli.Command {
	app := &cli.Command{}
	app.Name = "omada_exporter"
	app.Version = version
	app.Usage = "Prometheus Exporter for TP-Link Omada Controller SDN."
	app.EnableShellCompletion = true
	app.Suggest = true
	app.Authors = []any{
		"Charlie Haley <charlie-haley@users.noreply.github.com>",
		"Roman (RCooLeR) Derevianko <RCooLeR@users.noreply.github.com>",
	}
	app.Flags = []cli.Flag{
		&cli.StringFlag{Destination: &conf.Host, Name: "host", Value: "", Usage: "The hostname of the Omada Controller, including protocol.", Sources: cli.EnvVars("OMADA_HOST"), Validator: validateControllerURL},
		&cli.StringFlag{Destination: &conf.ClientId, Name: "client-id", Value: "", Usage: "ClientId for Omada OpenAPI client-credentials authentication.", Sources: cli.EnvVars("OMADA_CLIENT_ID")},
		&cli.StringFlag{Destination: &conf.SecretId, Name: "secret-id", Value: "", Usage: "SecretId for Omada OpenAPI client-credentials authentication.", Sources: cli.EnvVars("OMADA_SECRET_ID")},
		&cli.StringFlag{Destination: &conf.Username, Name: "username", Value: "", Usage: "Username of the Omada user you'd like to use to fetch metrics.", Sources: cli.EnvVars("OMADA_USER")},
		&cli.StringFlag{Destination: &conf.Password, Name: "password", Value: "", Usage: "Password for your Omada user.", Sources: cli.EnvVars("OMADA_PASS")},
		&cli.StringFlag{Destination: &conf.SystemType, Name: "system-type", Value: config.SystemTypeAuto, Usage: "Omada system type: auto, standard, or fusion.", Sources: cli.EnvVars("OMADA_SYSTEM_TYPE"), Validator: oneOf("system-type", config.SystemTypeAuto, config.SystemTypeStandard, config.SystemTypeFusion)},
		&cli.StringFlag{Destination: &conf.OpenAPIAuth, Name: "openapi-auth", Value: config.OpenAPIAuthAuto, Usage: "OpenAPI authentication mode: auto, client_credentials, web_session, or disabled.", Sources: cli.EnvVars("OMADA_OPENAPI_AUTH"), Validator: oneOf("openapi-auth", config.OpenAPIAuthAuto, config.OpenAPIAuthClientCredentials, config.OpenAPIAuthWebSession, config.OpenAPIAuthDisabled)},
		&cli.StringFlag{Destination: &conf.Port, Name: "port", Value: "9202", Usage: "Port on which to expose the Prometheus metrics.", Sources: cli.EnvVars("OMADA_PORT"), Validator: validatePort},
		&cli.StringFlag{Destination: &conf.Site, Name: "site", Value: "Default", Usage: "Omada site to scrape metrics from.", Sources: cli.EnvVars("OMADA_SITE")},
		&cli.StringFlag{Destination: &conf.LogLevel, Name: "log-level", Value: "error", Usage: "Application log level.", Sources: cli.EnvVars("LOG_LEVEL"), Validator: validateLogLevel},
		&cli.IntFlag{Destination: &conf.Timeout, Name: "timeout", Value: 15, Usage: "Timeout when making requests to the Omada Controller.", Sources: cli.EnvVars("OMADA_REQUEST_TIMEOUT"), Validator: positiveSeconds("timeout")},
		&cli.IntFlag{Destination: &conf.CacheTTL, Name: "cache-ttl", Value: 5, Usage: "Cache Omada API fetch results for this many seconds. Set 0 to disable.", Sources: cli.EnvVars("OMADA_CACHE_TTL"), Validator: nonNegativeSeconds("cache-ttl")},
		&cli.BoolFlag{Destination: &conf.Insecure, Name: "insecure", Value: false, Usage: "Whether to skip verifying the SSL certificate on the controller.", Sources: cli.EnvVars("OMADA_INSECURE")},
		&cli.BoolFlag{Destination: &conf.IncludePortActivityLabel, Name: "include-port-activity-label", Value: true, Usage: "Include the port_activity_label label on port metrics.", Sources: cli.EnvVars("OMADA_INCLUDE_PORT_ACTIVITY_LABEL")},
		&cli.BoolFlag{Destination: &conf.TrackPortMetrics, Name: "track-port-metrics", Value: true, Usage: "Export per-port metrics.", Sources: cli.EnvVars("OMADA_TRACK_PORT_METRICS")},
		&cli.BoolFlag{Destination: &conf.TrackClientMetrics, Name: "track-client-metrics", Value: true, Usage: "Export per-client metrics.", Sources: cli.EnvVars("OMADA_TRACK_CLIENT_METRICS")},
		&cli.BoolFlag{Destination: &conf.TrackInsightMetrics, Name: "track-insight-metrics", Value: false, Usage: "Export optional DPI insight metrics from Omada Web API.", Sources: cli.EnvVars("OMADA_TRACK_INSIGHT_METRICS")},
		&cli.IntFlag{Destination: &conf.InsightWindowSeconds, Name: "insight-window-seconds", Value: 86400, Usage: "DPI insight query window in seconds.", Sources: cli.EnvVars("OMADA_INSIGHT_WINDOW_SECONDS"), Validator: positiveSeconds("insight-window-seconds")},
		&cli.IntFlag{Destination: &conf.InsightApplicationLimit, Name: "insight-application-limit", Value: 50, Usage: "Maximum DPI application metric series to export. Set 0 to disable application metrics.", Sources: cli.EnvVars("OMADA_INSIGHT_APPLICATION_LIMIT"), Validator: nonNegativeInt("insight-application-limit")},
		&cli.BoolFlag{Destination: &conf.GoCollectorDisabled, Name: "disable-go-collector", Value: true, Usage: "Disable Go collector metrics.", Sources: cli.EnvVars("OMADA_DISABLE_GO_COLLECTOR")},
		&cli.BoolFlag{Destination: &conf.ProcessCollectorDisabled, Name: "disable-process-collector", Value: true, Usage: "Disable process collector metrics.", Sources: cli.EnvVars("OMADA_DISABLE_PROCESS_COLLECTOR")},
		&cli.BoolFlag{Destination: &conf.MQTTEnabled, Name: "mqtt-enabled", Value: false, Usage: "Enable Home Assistant MQTT discovery publishing.", Sources: cli.EnvVars("OMADA_MQTT_ENABLED")},
		&cli.StringFlag{Destination: &conf.MQTTBroker, Name: "mqtt-broker", Value: "tcp://localhost:1883", Usage: "MQTT broker URL, for example tcp://homeassistant.local:1883.", Sources: cli.EnvVars("OMADA_MQTT_BROKER")},
		&cli.StringFlag{Destination: &conf.MQTTUsername, Name: "mqtt-username", Value: "", Usage: "MQTT username.", Sources: cli.EnvVars("OMADA_MQTT_USER")},
		&cli.StringFlag{Destination: &conf.MQTTPassword, Name: "mqtt-password", Value: "", Usage: "MQTT password.", Sources: cli.EnvVars("OMADA_MQTT_PASS")},
		&cli.StringFlag{Destination: &conf.MQTTClientID, Name: "mqtt-client-id", Value: "omada_exporter", Usage: "MQTT client id.", Sources: cli.EnvVars("OMADA_MQTT_CLIENT_ID")},
		&cli.StringFlag{Destination: &conf.MQTTTopicPrefix, Name: "mqtt-topic-prefix", Value: "omada_exporter", Usage: "MQTT state topic prefix.", Sources: cli.EnvVars("OMADA_MQTT_TOPIC_PREFIX")},
		&cli.StringFlag{Destination: &conf.MQTTDiscoveryPrefix, Name: "mqtt-discovery-prefix", Value: "homeassistant", Usage: "Home Assistant MQTT discovery prefix.", Sources: cli.EnvVars("OMADA_MQTT_DISCOVERY_PREFIX")},
		&cli.IntFlag{Destination: &conf.MQTTInterval, Name: "mqtt-interval", Value: 60, Usage: "MQTT publish interval in seconds.", Sources: cli.EnvVars("OMADA_MQTT_INTERVAL"), Validator: positiveSeconds("mqtt-interval")},
		&cli.BoolFlag{Destination: &conf.MQTTRetain, Name: "mqtt-retain", Value: true, Usage: "Publish MQTT discovery and state messages as retained.", Sources: cli.EnvVars("OMADA_MQTT_RETAIN")},
		&cli.IntFlag{Destination: &conf.MQTTExpireAfter, Name: "mqtt-expire-after", Value: 180, Usage: "Home Assistant sensor expire_after value in seconds. Set 0 to disable.", Sources: cli.EnvVars("OMADA_MQTT_EXPIRE_AFTER"), Validator: nonNegativeSeconds("mqtt-expire-after")},
		&cli.StringFlag{Destination: &conf.MQTTTrackedClientMACs, Name: "mqtt-tracked-client-macs", Value: "", Usage: "Comma-separated client MAC addresses to publish Home Assistant device trackers for even when offline.", Sources: cli.EnvVars("OMADA_MQTT_TRACKED_CLIENT_MACS")},
	}
	for _, flag := range app.Flags {
		switch flag := flag.(type) {
		case *cli.StringFlag:
			flag.Local = true
			flag.OnlyOnce = true
		case *cli.IntFlag:
			flag.Local = true
			flag.OnlyOnce = true
		case *cli.BoolFlag:
			flag.Local = true
			flag.OnlyOnce = true
		}
	}
	app.Commands = []*cli.Command{
		{Name: "version", Aliases: []string{"v"}, Usage: "prints the current version.", Suggest: true,
			Action: func(_ context.Context, _ *cli.Command) error {
				fmt.Println(version)
				return nil
			}},
		{Name: "mdocs", Aliases: []string{"md"}, Usage: "prints the metric docs.", Suggest: true,
			Action: func(_ context.Context, _ *cli.Command) error {
				mdocs()
				return nil
			}},
	}
	app.Action = func(ctx context.Context, command *cli.Command) error {
		return runExporterWithConfig(ctx, command, conf)
	}
	return app
}

func validateControllerURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("host must be an absolute http or https URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("host URL must not contain a query or fragment")
	}
	if parsed.User != nil {
		return fmt.Errorf("host URL must not contain user credentials")
	}
	if port := parsed.Port(); port != "" {
		if err := validatePort(port); err != nil {
			return fmt.Errorf("invalid host URL port: %w", err)
		}
	}
	return nil
}

func validatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be an integer between 1 and 65535")
	}
	return nil
}

func validateLogLevel(value string) error {
	if _, err := zerolog.ParseLevel(value); err != nil {
		return fmt.Errorf("invalid log-level %q: %w", value, err)
	}
	return nil
}

func oneOf(name string, allowed ...string) func(string) error {
	return func(value string) error {
		for _, candidate := range allowed {
			if strings.EqualFold(value, candidate) {
				return nil
			}
		}
		return fmt.Errorf("%s must be one of %s", name, strings.Join(allowed, ", "))
	}
}

func nonNegativeInt(name string) func(int) error {
	return func(value int) error {
		if value < 0 {
			return fmt.Errorf("%s must not be negative", name)
		}
		return nil
	}
}

func positiveSeconds(name string) func(int) error {
	return boundedSeconds(name, false)
}

func nonNegativeSeconds(name string) func(int) error {
	return boundedSeconds(name, true)
}

func boundedSeconds(name string, allowZero bool) func(int) error {
	return func(value int) error {
		if value < 0 || (!allowZero && value == 0) {
			if allowZero {
				return fmt.Errorf("%s must not be negative", name)
			}
			return fmt.Errorf("%s must be greater than zero", name)
		}
		if int64(value) > maxSecondFlagValue {
			return fmt.Errorf("%s must not exceed %d seconds", name, maxSecondFlagValue)
		}
		return nil
	}
}
