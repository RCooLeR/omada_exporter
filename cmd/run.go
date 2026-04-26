package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/debugdump"
	"github.com/RCooLeR/omada_exporter/internal/hamqtt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

type healthState struct {
	ready atomic.Bool
}

func (h *healthState) livez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *healthState) readyz(w http.ResponseWriter, _ *http.Request) {
	if !h.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

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
	if conf.DumpResponsesDir != "" {
		if err := debugdump.DumpResponses(client, conf.DumpResponsesDir); err != nil {
			return err
		}
		if conf.DumpResponsesOnly {
			log.Info().Str("dir", conf.DumpResponsesDir).Msg("response dump complete")
			return nil
		}
	} else if conf.DumpResponsesOnly {
		return fmt.Errorf("dump-responses-only requires dump-responses-dir")
	}
	mux := http.NewServeMux()
	health := &healthState{}

	collectors := initCollectors(client)

	// register omada collectors
	for name, c := range collectors {
		prometheus.MustRegister(c)
		reg := prometheus.NewRegistry()
		reg.MustRegister(c)
		mux.Handle(fmt.Sprintf("/metrics/%s", name), promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	}

	if conf.MQTTEnabled {
		publisher, err := hamqtt.NewPublisher(client, collectors)
		if err != nil {
			return err
		}
		go func() {
			if err := publisher.Run(context.Background()); err != nil && err != context.Canceled {
				log.Error().Err(err).Msg("home assistant mqtt publisher stopped")
			}
		}()
	}

	mux.HandleFunc("/healthz", health.livez)
	mux.HandleFunc("/readyz", health.readyz)
	log.Info().Msg(fmt.Sprintf("listening on :%s", conf.Port))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
    <head>
	<title>Omada exporter<</title>
	</head>
    	<body>
			<h1>Omada exporter</h1>
			<p>
				<a href="/healthz">Health</a>
			</p>
			<p>
				<a href="/readyz">Ready</a>
			</p>
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

	mux.Handle("/metrics", promhttp.Handler())
	health.ready.Store(true)

	err = http.ListenAndServe(fmt.Sprintf(":%s", conf.Port), mux)
	if err != nil {
		return err
	}

	return nil
}
