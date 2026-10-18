// Package cmd defines the exporter CLI, flags, and informational commands.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/RCooLeR/omada_exporter/internal/app"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

var version = "dev"

// Start configures and runs the CLI application.
func Start() {
	command := newCLICommand(&config.Config{})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := command.Run(ctx, os.Args); err != nil {
		log.Fatal().Err(err).Msg("App failed to run")
	}
}

func newCLICommand(conf *config.Config) *cli.Command {
	command := &cli.Command{
		Name:                  "omada_exporter",
		Version:               version,
		Usage:                 "Prometheus Exporter for TP-Link Omada Controller SDN.",
		EnableShellCompletion: true,
		Suggest:               true,
		Authors: []any{
			"Charlie Haley <charlie-haley@users.noreply.github.com>",
			"Roman (RCooLeR) Derevianko <RCooLeR@users.noreply.github.com>",
		},
		Flags: configFlags(conf),
	}
	command.Commands = []*cli.Command{
		{Name: "version", Aliases: []string{"v"}, Usage: "prints the current version.", Suggest: true,
			Action: func(_ context.Context, command *cli.Command) error {
				_, err := fmt.Fprintln(command.Writer, version)
				return err
			}},
		{Name: "mdocs", Aliases: []string{"md"}, Usage: "prints the metric docs.", Suggest: true,
			Action: func(_ context.Context, command *cli.Command) error {
				return app.WriteMetricDocs(command.Writer)
			}},
	}
	command.Action = func(ctx context.Context, _ *cli.Command) error {
		return app.Run(ctx, conf)
	}
	return command
}
