package app

import (
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

func TestNetworkQualityOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		client := &session.Client{Config: &config.Config{TrackNetworkQualityMetrics: enabled}}
		_, exists := initCollectors(omada.NewClient(client))["network-quality"]
		if exists != enabled {
			t.Errorf("network-quality exists=%v enabled=%v", exists, enabled)
		}
	}
	if initCollectors(nil)["network-quality"] == nil {
		t.Fatal("metric documentation omits optional collector")
	}
}
