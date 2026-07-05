package config

const (
	SystemTypeAuto     = "auto"
	SystemTypeStandard = "standard"
	SystemTypeFusion   = "fusion"

	OpenAPIAuthAuto              = "auto"
	OpenAPIAuthClientCredentials = "client_credentials"
	OpenAPIAuthWebSession        = "web_session"
	OpenAPIAuthDisabled          = "disabled"
)

// Config stores the exporter configuration values.
type Config struct {
	Host                     string
	Username                 string
	Password                 string
	ClientId                 string
	SecretId                 string
	SystemType               string
	OpenAPIAuth              string
	Port                     string
	Site                     string
	LogLevel                 string
	Timeout                  int
	CacheTTL                 int
	Insecure                 bool
	IncludePortActivityLabel bool
	TrackPortMetrics         bool
	TrackClientMetrics       bool
	TrackInsightMetrics      bool
	InsightWindowSeconds     int
	InsightApplicationLimit  int
	GoCollectorDisabled      bool
	ProcessCollectorDisabled bool
	MQTTEnabled              bool
	MQTTBroker               string
	MQTTUsername             string
	MQTTPassword             string
	MQTTClientID             string
	MQTTTopicPrefix          string
	MQTTDiscoveryPrefix      string
	MQTTInterval             int
	MQTTRetain               bool
	MQTTExpireAfter          int
	MQTTTrackedClientMACs    string
}
