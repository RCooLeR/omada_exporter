package cmd

import (
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/urfave/cli/v3"
)

func configFlags(conf *config.Config) []cli.Flag {
	flags := []cli.Flag{
		&cli.BoolFlag{Destination: &conf.AllowDeviceControl, Name: "allow-device-control", Value: false, Usage: "Enable opt-in MQTT device reboot, Wi-Fi reconnect and PoE power-cycle controls. Requires MQTT and controller write permissions.", Sources: cli.EnvVars("OMADA_ALLOW_DEVICE_CONTROL")},
		&cli.StringFlag{Destination: &conf.BridgeID, Name: "bridge-id", Value: "", Usage: "Stable bridge namespace for isolated MQTT/HA identities and Prometheus labels. Empty preserves legacy identities.", Sources: cli.EnvVars("OMADA_BRIDGE_ID"), Validator: config.ValidateBridgeID},
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
		&cli.StringFlag{Destination: &conf.DumpResponsesDir, Name: "dump-responses-dir", Value: "", Usage: "Write raw diagnostic API responses into a new private subdirectory. Responses may contain sensitive network data.", Sources: cli.EnvVars("OMADA_DUMP_RESPONSES_DIR")},
		&cli.BoolFlag{Destination: &conf.DumpResponsesOnly, Name: "dump-responses-only", Value: false, Usage: "Exit after writing response dumps. Requires dump-responses-dir.", Sources: cli.EnvVars("OMADA_DUMP_RESPONSES_ONLY")},
		&cli.BoolFlag{Destination: &conf.IncludePortActivityLabel, Name: "include-port-activity-label", Value: true, Usage: "Include the port_activity_label label on port metrics.", Sources: cli.EnvVars("OMADA_INCLUDE_PORT_ACTIVITY_LABEL")},
		&cli.BoolFlag{Destination: &conf.TrackPortMetrics, Name: "track-port-metrics", Value: true, Usage: "Export per-port metrics.", Sources: cli.EnvVars("OMADA_TRACK_PORT_METRICS")},
		&cli.BoolFlag{Destination: &conf.TrackClientMetrics, Name: "track-client-metrics", Value: true, Usage: "Export per-client metrics.", Sources: cli.EnvVars("OMADA_TRACK_CLIENT_METRICS")},
		&cli.BoolFlag{Destination: &conf.TrackInsightMetrics, Name: "track-insight-metrics", Value: false, Usage: "Export optional DPI insight metrics from Omada Web API.", Sources: cli.EnvVars("OMADA_TRACK_INSIGHT_METRICS")},
		&cli.BoolFlag{Destination: &conf.TrackNetworkQualityMetrics, Name: "track-network-quality-metrics", Value: false, Usage: "Export optional AP wired-uplink packet counters and gateway WAN quality averages from Omada OpenAPI.", Sources: cli.EnvVars("OMADA_TRACK_NETWORK_QUALITY_METRICS")},
		&cli.IntFlag{Destination: &conf.NetworkQualityWindowSeconds, Name: "network-quality-window-seconds", Value: 86400, Usage: "Rolling query window for gateway WAN quality averages, in seconds.", Sources: cli.EnvVars("OMADA_NETWORK_QUALITY_WINDOW_SECONDS"), Validator: positiveSeconds("network-quality-window-seconds")},
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
	for _, flag := range flags {
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
	return flags
}
