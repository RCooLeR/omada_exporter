package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/urfave/cli/v3"
)

func TestNetworkQualityOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		client := &api.Client{Config: &config.Config{TrackNetworkQualityMetrics: enabled}}
		_, exists := initCollectors(client)["network-quality"]
		if exists != enabled {
			t.Errorf("network-quality exists=%v enabled=%v", exists, enabled)
		}
	}
	if initCollectors(nil)["network-quality"] == nil {
		t.Fatal("metric documentation omits optional collector")
	}
}

func TestNetworkQualityCLIFlagsAndEnvironment(t *testing.T) {
	for _, mode := range []string{"defaults", "flags", "env"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OMADA_TRACK_NETWORK_QUALITY_METRICS", "")
			t.Setenv("OMADA_NETWORK_QUALITY_WINDOW_SECONDS", "")
			args := []string{"omada_exporter"}
			wantWindow := 86400
			if mode == "flags" {
				args = append(args, "--track-network-quality-metrics", "--network-quality-window-seconds", "3600")
				wantWindow = 3600
			}
			if mode == "env" {
				t.Setenv("OMADA_TRACK_NETWORK_QUALITY_METRICS", "true")
				t.Setenv("OMADA_NETWORK_QUALITY_WINDOW_SECONDS", "3600")
				wantWindow = 3600
			}
			conf := &config.Config{}
			command := newCLICommand(conf)
			command.Writer = io.Discard
			command.ErrWriter = io.Discard
			command.Action = func(context.Context, *cli.Command) error { return nil }
			if err := command.Run(context.Background(), args); err != nil {
				t.Fatal(err)
			}
			if conf.TrackNetworkQualityMetrics != (mode != "defaults") || conf.NetworkQualityWindowSeconds != wantWindow {
				t.Fatalf("flag values=%v/%d", conf.TrackNetworkQualityMetrics, conf.NetworkQualityWindowSeconds)
			}
		})
	}
}
