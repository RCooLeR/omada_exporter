package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

func runExporter(c *cli.Context) error {
	// set log level
	level, err := zerolog.ParseLevel(conf.LogLevel)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(level)

	if conf.GoCollectorDisabled {
		// remove Go collector
		prometheus.Unregister(prometheus.NewGoCollector())
	}

	if conf.ProcessCollectorDisabled {
		// remove Process collector
		prometheus.Unregister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	}

	// check if host is properly formatted
	if strings.HasSuffix(conf.Host, "/") {
		// remove trailing slash if it exists
		conf.Host = strings.TrimRight(conf.Host, "/")
	}

	client, err := api.Configure(&conf)
	if err != nil {
		return err
	}

	// register omada collectors
	for name, c := range initCollectors(client) {
		prometheus.MustRegister(c)
		reg := prometheus.NewRegistry()
		reg.MustRegister(c)
		http.Handle(fmt.Sprintf("/metrics/%s", name), promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	}

	log.Info().Msg(fmt.Sprintf("listening on :%s", conf.Port))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
    <head>
	<title>Omada exporter<</title>
	</head>
    	<body>
			<h1>Omada exporter</h1>
			<p>
				<a href="/metrics">Metrics</a>
			</p>
			<p>
				<a href="/metrics/controller">Controller Metrics</a>
			</p>
			<p>
				<a href="/metrics/alert">Alert Metrics</a>
			</p>
			<p>
				<a href="/metrics/device">Device Metrics</a>
			</p>
			<p>
				<a href="/metrics/client">Client Metrics</a>
			</p>
			<p>
				<a href="/metrics/vpn">Vpn Metrics</a>
			</p>
			<p>
				<a href="/metrics/vpn">Vpn Stats Metrics</a>
			</p>
			<p>
				<a href="/metrics/vpn">ISP Metrics</a>
			</p>
    	</body>
    </html>`))
	})

	http.Handle("/metrics", promhttp.Handler())
	err = http.ListenAndServe(fmt.Sprintf(":%s", conf.Port), nil)
	if err != nil {
		return err
	}

	return nil
}
