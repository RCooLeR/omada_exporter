package cmd

import (
	"fmt"
	"os"

	"github.com/RCooLeR/omada_exporter/internal/config"
	log "github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

var version = "1.0.0"

var conf = config.Config{}

func Start() {
	app := cli.NewApp()
	app.Name = "omada_exporter"
	app.Version = version
	app.Usage = "Prometheus Exporter for TP-Link Omada Controller SDN."
	app.EnableBashCompletion = true
	app.Authors = []*cli.Author{
		{Name: "Charlie Haley", Email: "charlie-haley@users.noreply.github.com"},
		{Name: "Roman (RCooLeR) Derevianko", Email: "RCooLeR@users.noreply.github.com"},
	}
	app.Flags = []cli.Flag{
		&cli.StringFlag{Destination: &conf.Host, Required: true, Name: "host", Value: "", Usage: "The hostname of the Omada Controller, including protocol.", EnvVars: []string{"OMADA_HOST"}},
		&cli.StringFlag{Destination: &conf.ClientId, Required: true, Name: "client-id", Value: "", Usage: "ClientId for your Omada user.", EnvVars: []string{"OMADA_CLIENT_ID"}},
		&cli.StringFlag{Destination: &conf.SecretId, Required: true, Name: "secret-id", Value: "", Usage: "SecretId for your Omada user.", EnvVars: []string{"OMADA_SECRET_ID"}},
		&cli.StringFlag{Destination: &conf.Username, Required: true, Name: "username", Value: "", Usage: "Username of the Omada user you'd like to use to fetch metrics.", EnvVars: []string{"OMADA_USER"}},
		&cli.StringFlag{Destination: &conf.Password, Required: true, Name: "password", Value: "", Usage: "Password for your Omada user.", EnvVars: []string{"OMADA_PASS"}},
		&cli.StringFlag{Destination: &conf.Port, Name: "port", Value: "9202", Usage: "Port on which to expose the Prometheus metrics.", EnvVars: []string{"OMADA_PORT"}},
		&cli.StringFlag{Destination: &conf.Site, Name: "site", Value: "Default", Usage: "Omada site to scrape metrics from.", EnvVars: []string{"OMADA_SITE"}},
		&cli.StringFlag{Destination: &conf.LogLevel, Name: "log-level", Value: "error", Usage: "Application log level.", EnvVars: []string{"LOG_LEVEL"}},
		&cli.IntFlag{Destination: &conf.Timeout, Name: "timeout", Value: 15, Usage: "Timeout when making requests to the Omada Controller.", EnvVars: []string{"OMADA_REQUEST_TIMEOUT"}},
		&cli.BoolFlag{Destination: &conf.Insecure, Name: "insecure", Value: false, Usage: "Whether to skip verifying the SSL certificate on the controller.", EnvVars: []string{"OMADA_INSECURE"}},
		&cli.BoolFlag{Destination: &conf.GoCollectorDisabled, Name: "disable-go-collector", Value: true, Usage: "Disable Go collector metrics.", EnvVars: []string{"OMADA_DISABLE_GO_COLLECTOR"}},
		&cli.BoolFlag{Destination: &conf.ProcessCollectorDisabled, Name: "disable-process-collector", Value: true, Usage: "Disable process collector metrics.", EnvVars: []string{"OMADA_DISABLE_PROCESS_COLLECTOR"}},
	}
	app.Commands = []*cli.Command{
		{Name: "version", Aliases: []string{"v"}, Usage: "prints the current version.",
			Action: func(c *cli.Context) error {
				fmt.Println(version)
				os.Exit(0)
				return nil
			}},
		{Name: "mdocs", Aliases: []string{"md"}, Usage: "prints the metric docs.",
			Action: func(c *cli.Context) error {
				mdocs()
				os.Exit(0)
				return nil
			}},
	}
	app.Action = runExporter

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("App failed to run")
		os.Exit(1)
	}
}
