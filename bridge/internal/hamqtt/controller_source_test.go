package hamqtt

import (
	"context"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

func TestPublisherRequiresSettingsAndEnabledControlAPI(t *testing.T) {
	for _, source := range []controllerSource{nil, omada.NewClient(nil), &session.Client{}} {
		if _, err := NewPublisher(source, nil); err == nil {
			t.Fatal("publisher accepted missing settings")
		}
	}
	metadata := &session.Client{Config: &config.Config{AllowDeviceControl: true}}
	if _, err := NewPublisher(metadata, nil); err == nil {
		t.Fatal("enabled controls accepted a metadata-only source")
	}
	metadata.Config.AllowDeviceControl = false
	if _, err := NewPublisher(metadata, nil); err != nil {
		t.Fatalf("disabled controls should need only metadata: %v", err)
	}
}

func TestPublisherUsesSharedFacadeForControlsAndLiveSiteIdentity(t *testing.T) {
	shared := &session.Client{Config: &config.Config{AllowDeviceControl: true, BridgeID: "flat"}}
	shared.SetContextIDs("controller", "site-before")
	client := omada.NewClient(shared)
	p, err := NewPublisher(client, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.mqtt = newControlTestBroker()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // No controller or broker requests are needed to check the wiring.
	stop := p.startControls(ctx)
	defer stop()
	if p.controls.api != client {
		t.Fatal("controls did not reuse the shared domain client")
	}
	before := p.controlSiteKey()
	shared.SetContextIDs("controller", "site-after")
	if before == p.controlSiteKey() {
		t.Fatal("publisher captured stale site identity")
	}
}
