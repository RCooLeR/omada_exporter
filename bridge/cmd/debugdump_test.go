package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/urfave/cli/v3"
)

func TestCLIResponseDumpFlags(t *testing.T) {
	for _, source := range []string{"default", "flags", "environment"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("OMADA_DUMP_RESPONSES_DIR", "")
			t.Setenv("OMADA_DUMP_RESPONSES_ONLY", "false")
			conf := &config.Config{}
			command := newCLICommand(conf)
			command.Writer = io.Discard
			command.ErrWriter = io.Discard
			command.Action = func(context.Context, *cli.Command) error { return nil }
			args := []string{"omada_exporter"}
			dir := t.TempDir()
			if source == "flags" {
				args = append(args, "--dump-responses-dir", dir, "--dump-responses-only")
			}
			if source == "environment" {
				t.Setenv("OMADA_DUMP_RESPONSES_DIR", dir)
				t.Setenv("OMADA_DUMP_RESPONSES_ONLY", "true")
			}
			if err := command.Run(context.Background(), args); err != nil {
				t.Fatal(err)
			}
			if source == "default" {
				if conf.DumpResponsesDir != "" || conf.DumpResponsesOnly {
					t.Fatal("dump mode must be disabled by default")
				}
			} else if conf.DumpResponsesDir != dir || !conf.DumpResponsesOnly {
				t.Fatalf("dump flags not parsed: dir=%q only=%v", conf.DumpResponsesDir, conf.DumpResponsesOnly)
			}
		})
	}
}
