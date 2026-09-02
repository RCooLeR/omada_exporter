package collector

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type fixtureCollector struct {
	describe func(chan<- *prometheus.Desc)
	collect  func(chan<- prometheus.Metric)
}

func (c *fixtureCollector) Describe(ch chan<- *prometheus.Desc) {
	c.describe(ch)
}

func (c *fixtureCollector) Collect(ch chan<- prometheus.Metric) {
	c.collect(ch)
}

func TestBuildSiteToSiteTunnelIDsByVpnID(t *testing.T) {
	stats := []model.SiteToSiteVpnStats{
		{ID: "tunnel-1", VpnID: "vpn-1", Direction: "in"},
		{ID: "tunnel-1-other", VpnID: "vpn-1", Direction: "out"},
		{ID: "tunnel-1", VpnID: "vpn-1", Direction: "in"},
		{ID: "", VpnID: "vpn-2"},
		{ID: "tunnel-3", VpnID: ""},
		{ID: "tunnel-4", VpnID: "vpn-4"},
	}

	got := buildSiteToSiteTunnelIDsByVpnID(stats)

	if len(got) != 2 {
		t.Fatalf("expected 2 tunnel mappings, got %d", len(got))
	}
	if want := []string{"tunnel-1", "tunnel-1-other"}; !slices.Equal(got["vpn-1"], want) {
		t.Fatalf("vpn-1 tunnel ids = %q, want %q", got["vpn-1"], want)
	}
	if want := []string{"tunnel-4"}; !slices.Equal(got["vpn-4"], want) {
		t.Fatalf("vpn-4 tunnel ids = %q, want %q", got["vpn-4"], want)
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

func TestSiteToSiteVpnPeerSourceKeyPreservesDistinctPeersWithoutIDs(t *testing.T) {
	first := model.SiteToSiteVpnPeerStats{
		Name:       "peer-a",
		RemoteIP:   "192.0.2.2",
		Port:       51820,
		AllowedIPs: model.StringList{"10.0.0.0/24"},
	}
	second := first
	second.Name = "peer-b"
	second.AllowedIPs = model.StringList{"10.1.0.0/24"}

	if siteToSiteVpnPeerSourceKey(first, model.SiteToSiteVpnSummary{}) == siteToSiteVpnPeerSourceKey(second, model.SiteToSiteVpnSummary{}) {
		t.Fatal("distinct no-ID peers produced the same source key")
	}
	enriched := first
	enriched.Endpoint = "peer.example.test"
	if siteToSiteVpnPeerSourceKey(first, model.SiteToSiteVpnSummary{}) != siteToSiteVpnPeerSourceKey(enriched, model.SiteToSiteVpnSummary{}) {
		t.Fatal("optional endpoint enrichment changed a no-ID peer source key")
	}
}

func TestVpnTunnelMetricsCoalesceEnrichedSnapshots(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	first := model.VpnStats{
		VpnID:     "vpn-1",
		Name:      "Branch VPN",
		VpnType:   2,
		DownPkts:  10,
		DownBytes: 100,
		UpPkts:    20,
		UpBytes:   200,
	}
	enriched := first
	enriched.Endpoint = "gw.example.test"
	enriched.DownPkts = 15
	enriched.DownBytes = 150
	enriched.UpPkts = 25
	enriched.UpBytes = 250

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectVpnTunnelMetrics(ch, "Default", "site-1", []model.VpnStats{first, enriched})
		},
	})

	assertGaugeFamily(t, families, "omada_vpn_down_packets", []float64{15})
	assertGaugeFamily(t, families, "omada_vpn_down_bytes", []float64{150})
	assertGaugeFamily(t, families, "omada_vpn_up_packets", []float64{25})
	assertGaugeFamily(t, families, "omada_vpn_up_bytes", []float64{250})
	traffic := requireMetricFamily(t, families, "omada_vpn_down_bytes")
	if got := metricLabels(traffic.Metric[0])["endpoint"]; got != "gw.example.test" {
		t.Errorf("representative endpoint = %q, want enriched endpoint", got)
	}
}

func TestVpnTunnelMetricsPreserveDistinctSessionsForOneVPN(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	first := model.VpnStats{VpnID: "vpn-1", Name: "Branch VPN", VpnType: 2, RemoteIp: "192.0.2.2", DownBytes: 100}
	second := model.VpnStats{VpnID: "vpn-1", Name: "Branch VPN", VpnType: 2, RemoteIp: "192.0.2.3", DownBytes: 200}

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectVpnTunnelMetrics(ch, "Default", "site-1", []model.VpnStats{first, second})
		},
	})

	family := requireMetricFamily(t, families, "omada_vpn_down_bytes")
	if got := len(family.Metric); got != 2 {
		t.Fatalf("omada_vpn_down_bytes has %d series, want 2", got)
	}
	wantByRemoteIP := map[string]float64{"192.0.2.2": 100, "192.0.2.3": 200}
	for _, metric := range family.Metric {
		remoteIP := metricLabels(metric)["remote_ip"]
		want, ok := wantByRemoteIP[remoteIP]
		if !ok {
			t.Errorf("unexpected remote_ip %q", remoteIP)
			continue
		}
		if got := metric.GetGauge().GetValue(); got != want {
			t.Errorf("remote_ip %q value = %v, want %v", remoteIP, got, want)
		}
		delete(wantByRemoteIP, remoteIP)
	}
	if len(wantByRemoteIP) != 0 {
		t.Errorf("missing remote sessions: %v", wantByRemoteIP)
	}
}

func TestSiteToSiteVPNMetricsDeduplicateIdenticalRows(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{
		ID:          "vpn-1",
		Name:        "Branch VPN",
		VpnType:     2,
		SiteVpnType: 1,
	}
	row := model.SiteToSiteVpnStats{
		ID:             "tunnel-1",
		VpnID:          summary.ID,
		Name:           summary.Name,
		Direction:      "in",
		Spi:            1001,
		VpnType:        2,
		DownBytes:      123,
		UpBytes:        456,
		TotalRemoteNum: 1,
	}

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(
				ch,
				"Default",
				"site-1",
				[]model.SiteToSiteVpnStats{row, row},
				map[string]model.SiteToSiteVpnSummary{summary.ID: summary},
				nil,
				nil,
				map[string]struct{}{},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{123})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{456})
}

func TestSiteToSiteVPNMetricsKeepHighestDuplicateSnapshot(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{ID: "vpn-1", Name: "Branch VPN", VpnType: 2, SiteVpnType: 1}
	first := model.SiteToSiteVpnStats{ID: "tunnel-1", VpnID: summary.ID, Direction: "in", Spi: 1001, VpnType: 2, DownBytes: 100, UpBytes: 200}
	second := first
	second.DownBytes = 150
	second.UpBytes = 250

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(
				ch,
				"Default",
				"site-1",
				[]model.SiteToSiteVpnStats{first, second},
				map[string]model.SiteToSiteVpnSummary{summary.ID: summary},
				nil,
				nil,
				map[string]struct{}{},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{150})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{250})
}

func TestSiteToSiteVPNMetricsDeduplicateAcrossQueryTypes(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{ID: "vpn-1", Name: "Branch VPN", VpnType: 2, SiteVpnType: 1}
	first := model.SiteToSiteVpnStats{ID: "tunnel-1", VpnID: summary.ID, Direction: "in", Spi: 1001, VpnType: 2, DownBytes: 100, UpBytes: 200}
	second := first
	second.VpnType = 4
	second.DownBytes = 150
	second.UpBytes = 250

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(
				ch,
				"Default",
				"site-1",
				[]model.SiteToSiteVpnStats{first, second},
				map[string]model.SiteToSiteVpnSummary{summary.ID: summary},
				nil,
				nil,
				map[string]struct{}{},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{150})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{250})
}

func TestSiteToSiteVPNMetricsCoalesceEnrichedSnapshots(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{ID: "vpn-1", Name: "Branch VPN", VpnType: 2, SiteVpnType: 1}
	first := model.SiteToSiteVpnStats{ID: "tunnel-1", VpnID: summary.ID, Direction: "in", Spi: 1001, VpnType: 2, DownPkts: 10, DownBytes: 100, UpPkts: 20, UpBytes: 200, TotalRemoteNum: 1}
	enriched := first
	enriched.Endpoint = "gw.example.test"
	enriched.DownPkts = 15
	enriched.DownBytes = 150
	enriched.UpPkts = 25
	enriched.UpBytes = 250
	enriched.TotalRemoteNum = 2

	orders := []struct {
		name string
		rows []model.SiteToSiteVpnStats
	}{
		{name: "enrichment arrives last", rows: []model.SiteToSiteVpnStats{first, enriched}},
		{name: "enrichment arrives first", rows: []model.SiteToSiteVpnStats{enriched, first}},
	}
	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			families := gatherCollectorFixture(t, &fixtureCollector{
				describe: collector.Describe,
				collect: func(ch chan<- prometheus.Metric) {
					collector.collectSiteToSiteVpnMetrics(
						ch,
						"Default",
						"site-1",
						order.rows,
						map[string]model.SiteToSiteVpnSummary{summary.ID: summary},
						nil,
						nil,
						map[string]struct{}{},
					)
				},
			})

			assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{150})
			assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{250})
			assertGaugeFamily(t, families, "omada_vpn_down_packets", []float64{15})
			assertGaugeFamily(t, families, "omada_vpn_up_packets", []float64{25})
			assertGaugeFamily(t, families, "omada_site_to_site_vpn_total_peers", []float64{2})
			traffic := requireMetricFamily(t, families, "omada_site_to_site_vpn_down_bytes")
			if got := metricLabels(traffic.Metric[0])["endpoint"]; got != "gw.example.test" {
				t.Errorf("representative endpoint = %q, want enriched endpoint", got)
			}
			totalPeers := requireMetricFamily(t, families, "omada_site_to_site_vpn_total_peers")
			if got := metricLabels(totalPeers.Metric[0])["endpoint"]; got != "gw.example.test" {
				t.Errorf("total-peers representative endpoint = %q, want enriched endpoint", got)
			}
		})
	}
}

func TestSiteToSiteVPNMetricsCoalesceEnrichedSnapshotsWithoutIDs(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	first := model.SiteToSiteVpnStats{
		Name:           "Legacy VPN",
		Direction:      "in",
		Spi:            1001,
		VpnType:        2,
		DownBytes:      100,
		UpBytes:        200,
		TotalRemoteNum: 1,
	}
	enriched := first
	enriched.Endpoint = "gw.example.test"
	enriched.DownBytes = 150
	enriched.UpBytes = 250
	enriched.TotalRemoteNum = 2

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(
				ch,
				"Default",
				"site-1",
				[]model.SiteToSiteVpnStats{first, enriched},
				nil,
				nil,
				nil,
				map[string]struct{}{},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{150})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{250})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_total_peers", []float64{2})
	traffic := requireMetricFamily(t, families, "omada_site_to_site_vpn_down_bytes")
	if got := metricLabels(traffic.Metric[0])["endpoint"]; got != "gw.example.test" {
		t.Errorf("representative endpoint = %q, want enriched endpoint", got)
	}
}

func TestSiteToSiteVpnStatsSourceKeyPreservesTypeAndModeWithoutIDs(t *testing.T) {
	first := model.SiteToSiteVpnStats{Name: "Legacy VPN", Direction: "in", Spi: 1001, VpnMode: 0, VpnType: 2}
	differentType := first
	differentType.VpnType = 4
	differentMode := first
	differentMode.VpnMode = 1

	firstKey := siteToSiteVpnStatsSourceKey(first)
	if firstKey == siteToSiteVpnStatsSourceKey(differentType) {
		t.Fatal("different no-ID VPN types produced the same source key")
	}
	if firstKey == siteToSiteVpnStatsSourceKey(differentMode) {
		t.Fatal("different no-ID VPN modes produced the same source key")
	}
}

func TestSiteToSiteVPNMetricsPreserveDistinctNamesWithoutIDs(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	rows := []model.SiteToSiteVpnStats{
		{Name: "Legacy VPN A", Direction: "in", Spi: 1001, VpnType: 2, DownBytes: 100},
		{Name: "Legacy VPN B", Direction: "in", Spi: 1001, VpnType: 2, DownBytes: 200},
	}

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(ch, "Default", "site-1", rows, nil, nil, nil, map[string]struct{}{})
		},
	})

	family := requireMetricFamily(t, families, "omada_site_to_site_vpn_down_bytes")
	if got := len(family.Metric); got != 2 {
		t.Fatalf("omada_site_to_site_vpn_down_bytes has %d series, want 2", got)
	}
	wantByName := map[string]float64{"Legacy VPN A": 100, "Legacy VPN B": 200}
	for _, metric := range family.Metric {
		name := metricLabels(metric)["name"]
		if got, want := metric.GetGauge().GetValue(), wantByName[name]; got != want {
			t.Errorf("VPN %q value = %v, want %v", name, got, want)
		}
		delete(wantByName, name)
	}
	if len(wantByName) != 0 {
		t.Errorf("missing VPNs: %v", wantByName)
	}
}

func TestSiteToSiteVPNMetricsPreserveRowsWithoutVpnID(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	rows := []model.SiteToSiteVpnStats{
		{ID: "tunnel-1", Name: "Legacy VPN", Direction: "in", Spi: 1001, VpnType: 2, DownBytes: 100, UpBytes: 20},
		{ID: "tunnel-2", Name: "Legacy VPN", Direction: "out", Spi: 1002, VpnType: 2, DownBytes: 40, UpBytes: 80},
	}

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(ch, "Default", "site-1", rows, nil, nil, nil, map[string]struct{}{})
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{140})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{100})
	if got := len(requireMetricFamily(t, families, "omada_site_to_site_vpn_total_peers").Metric); got != 2 {
		t.Fatalf("omada_site_to_site_vpn_total_peers has %d series, want 2", got)
	}
}

func TestSiteToSiteVPNMetricsAggregateDistinctTunnelRows(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{
		ID:          "vpn-1",
		Name:        "Branch VPN",
		VpnType:     2,
		SiteVpnType: 1,
	}
	rows := []model.SiteToSiteVpnStats{
		{
			ID:             "tunnel-in",
			VpnID:          summary.ID,
			Name:           summary.Name,
			Direction:      "in",
			Spi:            1001,
			VpnType:        2,
			DownPkts:       100,
			DownBytes:      100,
			UpPkts:         20,
			UpBytes:        20,
			TotalRemoteNum: 1,
		},
		{
			ID:             "tunnel-out",
			VpnID:          summary.ID,
			Name:           summary.Name,
			Direction:      "out",
			Spi:            1002,
			VpnType:        2,
			DownPkts:       40,
			DownBytes:      40,
			UpPkts:         80,
			UpBytes:        80,
			TotalRemoteNum: 1,
		},
	}

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(
				ch,
				"Default",
				"site-1",
				rows,
				map[string]model.SiteToSiteVpnSummary{summary.ID: summary},
				nil,
				nil,
				map[string]struct{}{},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{140})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{100})
	assertGaugeFamily(t, families, "omada_vpn_down_packets", []float64{140})
	assertGaugeFamily(t, families, "omada_vpn_up_packets", []float64{100})

	totalPeers := requireMetricFamily(t, families, "omada_site_to_site_vpn_total_peers")
	if got := len(totalPeers.Metric); got != 2 {
		t.Fatalf("omada_site_to_site_vpn_total_peers has %d series, want 2", got)
	}
	wantTunnels := map[string]string{
		"tunnel-in":  "in",
		"tunnel-out": "out",
	}
	for _, metric := range totalPeers.Metric {
		labels := metricLabels(metric)
		tunnelID := labels["tunnel_id"]
		wantDirection, ok := wantTunnels[tunnelID]
		if !ok {
			t.Errorf("unexpected total_peers tunnel_id %q", tunnelID)
			continue
		}
		if got := labels["direction"]; got != wantDirection {
			t.Errorf("total_peers direction for %q = %q, want %q", tunnelID, got, wantDirection)
		}
		delete(wantTunnels, tunnelID)
	}
	if len(wantTunnels) != 0 {
		t.Errorf("missing total_peers tunnel series: %v", wantTunnels)
	}
}

func TestSiteToSiteVPNMetricsEmitPeerAggregateOnce(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{ID: "vpn-1", Name: "Branch VPN", VpnType: 2, SiteVpnType: 1}
	stats := []model.SiteToSiteVpnStats{
		{ID: "tunnel-1", VpnID: summary.ID, Direction: "in", Spi: 1001, VpnType: 2},
		{ID: "tunnel-2", VpnID: summary.ID, Direction: "out", Spi: 1002, VpnType: 2, LocalIP: "192.0.2.1"},
	}
	peer := model.SiteToSiteVpnPeerStats{
		ID:        "row-1",
		VpnID:     "peer-1",
		RemoteIP:  "192.0.2.2",
		DownBytes: 700,
		UpBytes:   900,
	}

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnMetrics(
				ch,
				"Default",
				"site-1",
				stats,
				map[string]model.SiteToSiteVpnSummary{summary.ID: summary},
				map[string][]model.SiteToSiteVpnPeerStats{summary.ID: {peer, peer}},
				map[string]bool{summary.ID: true},
				map[string]struct{}{},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{700})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{900})
	traffic := requireMetricFamily(t, families, "omada_site_to_site_vpn_down_bytes")
	if got := metricLabels(traffic.Metric[0])["local_ip"]; got != "192.0.2.1" {
		t.Errorf("representative local_ip = %q, want the populated label", got)
	}
}

func TestSiteToSiteVPNMetricsFallBackForIncompletePeerCounters(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{ID: "vpn-1", Name: "Branch VPN", VpnType: 2, SiteVpnType: 1}
	stats := []model.SiteToSiteVpnStats{
		{ID: "tunnel-1", VpnID: summary.ID, Direction: "in", Spi: 1001, VpnType: 2, DownBytes: 100, UpBytes: 20},
		{ID: "tunnel-2", VpnID: summary.ID, Direction: "out", Spi: 1002, VpnType: 2, DownBytes: 40, UpBytes: 80},
	}
	peer := model.SiteToSiteVpnPeerStats{ID: "row-1", VpnID: "peer-1", DownBytes: 700}

	tests := []struct {
		name             string
		peerDataComplete bool
		wantDown         float64
		wantUp           float64
	}{
		{name: "complete peer response falls back per missing direction", peerDataComplete: true, wantDown: 700, wantUp: 100},
		{name: "partial peer response keeps raw tunnel totals", peerDataComplete: false, wantDown: 140, wantUp: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			families := gatherCollectorFixture(t, &fixtureCollector{
				describe: collector.Describe,
				collect: func(ch chan<- prometheus.Metric) {
					collector.collectSiteToSiteVpnMetrics(
						ch,
						"Default",
						"site-1",
						stats,
						map[string]model.SiteToSiteVpnSummary{summary.ID: summary},
						map[string][]model.SiteToSiteVpnPeerStats{summary.ID: {peer}},
						map[string]bool{summary.ID: tt.peerDataComplete},
						map[string]struct{}{},
					)
				},
			})

			assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{tt.wantDown})
			assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{tt.wantUp})
		})
	}
}

func TestSiteToSiteVPNPeerMetricsDeduplicateIdenticalRows(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{
		ID:          "vpn-1",
		Name:        "Branch VPN",
		VpnType:     4,
		SiteVpnType: 1,
	}
	status := int8(1)
	downPackets := int64(7)
	upPackets := int64(9)
	peer := model.SiteToSiteVpnPeerStats{
		ID:        "peer-row-1",
		VpnID:     "peer-1",
		Name:      "Remote peer",
		RemoteIP:  "192.0.2.2",
		LocalIP:   "192.0.2.1",
		DownPkts:  &downPackets,
		DownBytes: 700,
		UpPkts:    &upPackets,
		UpBytes:   900,
		LoginTime: 1_700_000_000,
		Port:      51820,
		Status:    &status,
	}

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnPeerMetrics(
				ch,
				"Default",
				"site-1",
				[]model.SiteToSiteVpnSummary{summary},
				map[string][]model.SiteToSiteVpnPeerStats{
					summary.ID: {peer, peer},
				},
			)
		},
	})

	for _, want := range []struct {
		name  string
		value float64
	}{
		{"omada_site_to_site_vpn_peer_status", 1},
		{"omada_site_to_site_vpn_peer_down_packets", 7},
		{"omada_site_to_site_vpn_peer_down_bytes", 700},
		{"omada_site_to_site_vpn_peer_up_packets", 9},
		{"omada_site_to_site_vpn_peer_up_bytes", 900},
		{"omada_site_to_site_vpn_peer_login_timestamp", 1_700_000_000},
	} {
		t.Run(want.name, func(t *testing.T) {
			assertGaugeFamily(t, families, want.name, []float64{want.value})
		})
	}
}

func TestSiteToSiteVPNPeerMetricsCoalesceEnrichedSnapshots(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{ID: "vpn-1", Name: "Branch VPN", VpnType: 4, SiteVpnType: 1}
	statusOffline := int8(0)
	statusOnline := int8(1)
	first := model.SiteToSiteVpnPeerStats{
		ID:        "peer-row-1",
		VpnID:     "peer-1",
		RemoteIP:  "192.0.2.2",
		DownBytes: 700,
		UpBytes:   900,
		Status:    &statusOffline,
	}
	enriched := first
	enriched.Endpoint = "peer.example.test"
	enriched.DownBytes = 800
	enriched.UpBytes = 950
	enriched.Status = &statusOnline

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnPeerMetrics(
				ch,
				"Default",
				"site-1",
				[]model.SiteToSiteVpnSummary{summary},
				map[string][]model.SiteToSiteVpnPeerStats{summary.ID: {first, enriched}},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_peer_status", []float64{1})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_peer_down_bytes", []float64{800})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_peer_up_bytes", []float64{950})
	traffic := requireMetricFamily(t, families, "omada_site_to_site_vpn_peer_down_bytes")
	if got := metricLabels(traffic.Metric[0])["endpoint"]; got != "peer.example.test" {
		t.Errorf("representative endpoint = %q, want enriched endpoint", got)
	}
}

func TestSiteToSiteVPNPeerMetricsNormalizeSummaryInheritedLabels(t *testing.T) {
	collector := NewVpnStatsCollector(nil)
	summary := model.SiteToSiteVpnSummary{
		ID:         "vpn-1",
		Name:       "Branch VPN",
		VpnType:    4,
		AllowedIPs: model.StringList{"10.0.0.0/24"},
	}
	first := model.SiteToSiteVpnPeerStats{Name: "Remote peer", RemoteIP: "192.0.2.2", Port: 51820, DownBytes: 100, UpBytes: 200}
	explicit := first
	explicit.AllowedIPs = model.StringList{"10.0.0.0/24"}
	explicit.DownBytes = 150
	explicit.UpBytes = 250

	families := gatherCollectorFixture(t, &fixtureCollector{
		describe: collector.Describe,
		collect: func(ch chan<- prometheus.Metric) {
			collector.collectSiteToSiteVpnPeerMetrics(
				ch,
				"Default",
				"site-1",
				[]model.SiteToSiteVpnSummary{summary},
				map[string][]model.SiteToSiteVpnPeerStats{summary.ID: {first, explicit}},
			)
		},
	})

	assertGaugeFamily(t, families, "omada_site_to_site_vpn_peer_down_bytes", []float64{150})
	assertGaugeFamily(t, families, "omada_site_to_site_vpn_peer_up_bytes", []float64{250})
	traffic := requireMetricFamily(t, families, "omada_site_to_site_vpn_peer_down_bytes")
	if got := metricLabels(traffic.Metric[0])["allowed_ips"]; got != "10.0.0.0/24" {
		t.Errorf("allowed_ips = %q, want summary-normalized value", got)
	}
}

func TestAggregateSiteToSitePeerBytesDeduplicatesSnapshotsAndSumsPeers(t *testing.T) {
	firstSnapshot := model.SiteToSiteVpnPeerStats{
		ID:        "row-1",
		VpnID:     "peer-1",
		RemoteIP:  "192.0.2.2",
		DownBytes: 100,
		UpBytes:   200,
	}
	newerSnapshot := firstSnapshot
	newerSnapshot.Endpoint = "peer.example.test"
	newerSnapshot.DownBytes = 150
	newerSnapshot.UpBytes = 250
	secondPeer := model.SiteToSiteVpnPeerStats{
		ID:        "row-2",
		VpnID:     "peer-2",
		RemoteIP:  "192.0.2.3",
		DownBytes: 10,
		UpBytes:   20,
	}

	downBytes, upBytes := aggregateSiteToSitePeerBytes([]model.SiteToSiteVpnPeerStats{
		firstSnapshot,
		newerSnapshot,
		secondPeer,
	}, model.SiteToSiteVpnSummary{})

	if downBytes != 160 {
		t.Errorf("down bytes = %d, want 160", downBytes)
	}
	if upBytes != 270 {
		t.Errorf("up bytes = %d, want 270", upBytes)
	}
}

func TestVpnStatsCollectorGatherDeduplicatesOverlappingQueries(t *testing.T) {
	tests := []struct {
		name           string
		failSecondPeer bool
		wantDownBytes  float64
		wantUpBytes    float64
	}{
		{name: "complete peer totals replace raw counters", wantDownBytes: 1000, wantUpBytes: 1300},
		{name: "partial peer totals fall back to raw counters", failSecondPeer: true, wantDownBytes: 140, wantUpBytes: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var siteToSiteRequests atomic.Int32
			var peerRequests atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/info":
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"omadacId":"cid"}}`))
				case "/cid/api/v2/loginStatus":
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"login":true}}`))
				case "/cid/api/v2/users/current":
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"privilege":{"sites":[{"name":"Default","key":"site-id"}]}}}`))
				case "/openapi/authorize/token":
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"accessToken":"access-token","refreshToken":"refresh-token","expiresIn":3600}}`))
				case "/openapi/v1/cid/sites/site-id/setting/vpn/stats/tunnel":
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":0,"currentPage":1,"currentSize":0,"data":[]}}`))
				case "/openapi/v2/cid/sites/site-id/vpn/site-to-site-vpns":
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":1,"currentPage":1,"currentSize":1,"data":[{"id":"vpn-1","name":"Branch VPN","vpnType":2,"siteVpnType":1}]}}`))
				case "/openapi/v1/cid/sites/site-id/setting/vpn/stats/s2s":
					vpnType := req.URL.Query().Get("filters.vpnType")
					if vpnType != "2" && vpnType != "4" {
						t.Errorf("unexpected vpnType filter %q", vpnType)
					}
					siteToSiteRequests.Add(1)
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":2,"currentPage":1,"currentSize":2,"data":[{"id":"tunnel-1","vpnId":"vpn-1","name":"Branch VPN","direction":"in","spi":1001,"vpnType":2,"downBytes":100,"upBytes":20,"totalRemoteNum":1},{"id":"tunnel-2","vpnId":"vpn-1","name":"Branch VPN","direction":"out","spi":1002,"vpnType":2,"downBytes":40,"upBytes":80,"totalRemoteNum":1}]}}`))
				case "/openapi/v1/cid/sites/site-id/setting/vpn/stats/s2s/tunnel-1/peer":
					peerRequests.Add(1)
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":1,"currentPage":1,"currentSize":1,"data":[{"id":"peer-row-1","vpnId":"peer-1","downBytes":700,"upBytes":900}]}}`))
				case "/openapi/v1/cid/sites/site-id/setting/vpn/stats/s2s/tunnel-2/peer":
					peerRequests.Add(1)
					if tt.failSecondPeer {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte(`{"errorCode":-1,"msg":"peer endpoint unavailable"}`))
						return
					}
					_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":1,"currentPage":1,"currentSize":1,"data":[{"id":"peer-row-2","vpnId":"peer-2","downBytes":300,"upBytes":400}]}}`))
				default:
					t.Errorf("unexpected request path %s", req.URL.Path)
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			apiClient, err := api.Configure(&config.Config{
				Host:        server.URL,
				Username:    "user",
				Password:    "pass",
				ClientId:    "client-id",
				SecretId:    "client-secret",
				SystemType:  config.SystemTypeStandard,
				OpenAPIAuth: config.OpenAPIAuthClientCredentials,
				Site:        "Default",
				Timeout:     5,
			})
			if err != nil {
				t.Fatalf("configure API client: %v", err)
			}

			registry := prometheus.NewPedanticRegistry()
			if err := registry.Register(NewVpnStatsCollector(apiClient)); err != nil {
				t.Fatalf("register VPN collector: %v", err)
			}
			families, err := registry.Gather()
			if err != nil {
				t.Fatalf("gather VPN collector: %v", err)
			}

			assertGaugeFamily(t, families, "omada_site_to_site_vpn_down_bytes", []float64{tt.wantDownBytes})
			assertGaugeFamily(t, families, "omada_site_to_site_vpn_up_bytes", []float64{tt.wantUpBytes})
			assertGaugeFamily(t, families, "omada_site_to_site_vpn_total_peers", []float64{1, 1})
			if got := siteToSiteRequests.Load(); got != 2 {
				t.Errorf("site-to-site stats requests = %d, want 2", got)
			}
			if got := peerRequests.Load(); got != 2 {
				t.Errorf("peer stats requests = %d, want both unique tunnel requests", got)
			}
		})
	}
}

func gatherCollectorFixture(t *testing.T, collector prometheus.Collector) []*dto.MetricFamily {
	t.Helper()

	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatalf("register collector fixture: %v", err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather collector fixture: %v", err)
	}
	return families
}

func assertGaugeFamily(t *testing.T, families []*dto.MetricFamily, name string, want []float64) {
	t.Helper()

	family := requireMetricFamily(t, families, name)
	if got := len(family.Metric); got != len(want) {
		t.Fatalf("%s has %d series, want %d", name, got, len(want))
	}
	for i, metric := range family.Metric {
		if metric.Gauge == nil {
			t.Fatalf("%s series %d is not a gauge", name, i)
		}
		if got := metric.GetGauge().GetValue(); got != want[i] {
			t.Errorf("%s series %d = %v, want %v", name, i, got, want[i])
		}
	}
}

func requireMetricFamily(t *testing.T, families []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()

	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("metric family %q was not gathered", name)
	return nil
}

func metricLabels(metric *dto.Metric) map[string]string {
	labels := make(map[string]string, len(metric.Label))
	for _, label := range metric.Label {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}
