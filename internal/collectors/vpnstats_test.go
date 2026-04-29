package collector

import (
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

func TestBuildSiteToSiteTunnelIDByVpnID(t *testing.T) {
	stats := []model.SiteToSiteVpnStats{
		{ID: "tunnel-1", VpnID: "vpn-1", Direction: "in"},
		{ID: "tunnel-1-other", VpnID: "vpn-1", Direction: "out"},
		{ID: "", VpnID: "vpn-2"},
		{ID: "tunnel-3", VpnID: ""},
		{ID: "tunnel-4", VpnID: "vpn-4"},
	}

	got := buildSiteToSiteTunnelIDByVpnID(stats)

	if len(got) != 2 {
		t.Fatalf("expected 2 tunnel mappings, got %d", len(got))
	}
	if got["vpn-1"] != "tunnel-1" {
		t.Fatalf("expected vpn-1 to map to first tunnel id, got %q", got["vpn-1"])
	}
	if got["vpn-4"] != "tunnel-4" {
		t.Fatalf("expected vpn-4 to map to tunnel-4, got %q", got["vpn-4"])
	}
}

func TestSiteToSitePeerID(t *testing.T) {
	tests := []struct {
		name string
		item model.SiteToSiteVpnPeerStats
		want string
	}{
		{
			name: "prefers peer vpnId",
			item: model.SiteToSiteVpnPeerStats{ID: "row-id", VpnID: "peer-id"},
			want: "peer-id",
		},
		{
			name: "falls back to id",
			item: model.SiteToSiteVpnPeerStats{ID: "row-id"},
			want: "row-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := siteToSitePeerID(tt.item); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
