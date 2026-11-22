package cmd

import (
	"io"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestCLIMSPFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		env         map[string]string
		wantSiteSet bool
	}{
		{name: "defaults", args: nil, wantSiteSet: false},
		{name: "site flag", args: []string{"--site", "Default"}, wantSiteSet: true},
		{name: "site env", env: map[string]string{"OMADA_SITE": "Default"}, wantSiteSet: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			conf := &config.Config{}
			command := newCLICommand(conf)
			command.Writer, command.ErrWriter = io.Discard, io.Discard
			args := append(append([]string{"omada_exporter", "--system-type", "msp"}, tc.args...), "version")
			if err := command.Run(t.Context(), args); err != nil {
				t.Fatal(err)
			}
			if conf.SiteSet != tc.wantSiteSet {
				t.Fatalf("SiteSet = %v, want %v", conf.SiteSet, tc.wantSiteSet)
			}
			if conf.SystemType != config.SystemTypeMSP || conf.MSPCustomers != "all" ||
				conf.MSPDiscoveryInterval != config.DefaultMSPDiscoveryInterval || conf.MSPScrapeConcurrency != config.DefaultMSPScrapeConcurrency {
				t.Fatalf("MSP defaults = %q %q %d %d", conf.SystemType, conf.MSPCustomers, conf.MSPDiscoveryInterval, conf.MSPScrapeConcurrency)
			}
		})
	}
}

func TestCLIRejectsInvalidMSPConcurrency(t *testing.T) {
	command := newCLICommand(&config.Config{})
	command.Writer, command.ErrWriter = io.Discard, io.Discard
	if err := command.Run(t.Context(), []string{"omada_exporter", "--msp-scrape-concurrency", "0", "version"}); err == nil {
		t.Fatal("expected a validation error")
	}
}
