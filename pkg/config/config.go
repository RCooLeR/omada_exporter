package config

type Config struct {
	Host                     string
	Username                 string
	Password                 string
	ClientId                 string
	SecretId                 string
	Port                     string
	Site                     string
	LogLevel                 string
	Timeout                  int
	Insecure                 bool
	GoCollectorDisabled      bool
	ProcessCollectorDisabled bool
}
