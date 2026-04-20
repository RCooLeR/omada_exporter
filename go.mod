module github.com/RCooLeR/omada_exporter

go 1.26

replace github.com/RCooLeR/omada_exporte => ./

replace github.com/charlie-haley/omada_exporter => ./

require (
	github.com/eclipse/paho.mqtt.golang v1.5.1
	github.com/goki/ki v1.1.17
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/rs/zerolog v1.35.0
	github.com/urfave/cli/v2 v2.27.7
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.21 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
