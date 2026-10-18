package collector

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func qualityTestCollector() *NetworkQualityCollector {
	client := &session.Client{Config: &config.Config{Site: "test", TrackNetworkQualityMetrics: true, NetworkQualityWindowSeconds: 3600}}
	client.SetContextIDs("controller", "site-id")
	c := NewNetworkQualityCollector(omada.NewClient(client))
	c.getDevices = func() ([]model.DevicesInterface, error) {
		return []model.DevicesInterface{
			&model.AccessPoint{Device: model.Device{Mac: "ap", Name: "AP", Type: "ap", Status: 14}},
			&model.Gateway{Device: model.Device{Mac: "gw", Name: "Gateway", Type: "gateway", Status: 14}},
		}, nil
	}
	c.getAPUplink = func(string) (*model.APUplinkStats, error) {
		return &model.APUplinkStats{UpPackets: new(uint64(100)), DownPackets: new(uint64(200)), UpDropPackets: new(uint64(0)), DownDropPackets: new(uint64(3)), UpErrorPackets: new(uint64(0)), DownErrorPackets: new(uint64(1))}, nil
	}
	c.getWANQuality = func(string, int) (*model.WANQuality, error) {
		return &model.WANQuality{LatencyMilliseconds: new(250.0), JitterMilliseconds: new(0.0), PacketLossPercent: new(0.0), MOS: new(4.2)}, nil
	}
	return c
}

func gatherQuality(t *testing.T, c prometheus.Collector) map[string]*dto.MetricFamily {
	t.Helper()
	r := prometheus.NewPedanticRegistry()
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	families, err := r.Gather()
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]*dto.MetricFamily{}
	for _, f := range families {
		result[f.GetName()] = f
	}
	return result
}

func TestNetworkQualityValuesTypesAndLabels(t *testing.T) {
	c := qualityTestCollector()
	c.getWANQuality = func(mac string, window int) (*model.WANQuality, error) {
		if mac != "gw" || window != 3600 {
			t.Errorf("WAN request = %q/%d", mac, window)
		}
		return &model.WANQuality{LatencyMilliseconds: new(250.0), JitterMilliseconds: new(0.0), PacketLossPercent: new(0.0), MOS: new(4.2)}, nil
	}
	families := gatherQuality(t, c)
	for name, want := range map[string]float64{
		"omada_gateway_wan_quality_latency_average_seconds":        .25,
		"omada_gateway_wan_quality_jitter_average_seconds":         0,
		"omada_gateway_wan_quality_packet_loss_average_percentage": 0,
		"omada_gateway_wan_quality_mos_average":                    4.2,
		"omada_gateway_wan_quality_window_seconds":                 3600,
	} {
		f := families[name]
		if f == nil || f.GetType() != dto.MetricType_GAUGE || len(f.Metric) != 1 || f.Metric[0].Gauge.GetValue() != want {
			t.Errorf("%s = %v; want gauge %v", name, f, want)
		}
	}
	for name, expected := range map[string]map[string]float64{
		"omada_ap_uplink_packets_total":         {"up": 100, "down": 200},
		"omada_ap_uplink_dropped_packets_total": {"up": 0, "down": 3},
		"omada_ap_uplink_error_packets_total":   {"up": 0, "down": 1},
	} {
		f := families[name]
		if f == nil || f.GetType() != dto.MetricType_COUNTER || len(f.Metric) != 2 {
			t.Fatalf("%s = %v", name, f)
		}
		for _, m := range f.Metric {
			labels := map[string]string{}
			for _, l := range m.Label {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["device_mac"] != "ap" || labels["site_id"] != "site-id" || m.Counter.GetValue() != expected[labels["direction"]] {
				t.Errorf("bad packet labels/value: %v", m)
			}
		}
	}
}

func TestNetworkQualityUnsupportedAndFailuresDoNotProduceZeros(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsupported", true: "error"}[failure], func(t *testing.T) {
			c := qualityTestCollector()
			c.getWANQuality = func(string, int) (*model.WANQuality, error) {
				if failure {
					return nil, errors.New("test")
				}
				return nil, nil
			}
			c.getAPUplink = func(string) (*model.APUplinkStats, error) {
				if failure {
					return nil, errors.New("test")
				}
				return nil, nil
			}
			families := gatherQuality(t, c)
			if len(families) != 2 {
				t.Fatalf("only status families expected, got %v", families)
			}
			for _, m := range families["omada_network_quality_available"].Metric {
				if m.Gauge.GetValue() != 0 {
					t.Error("unavailable measurement reported available")
				}
			}
			for _, m := range families["omada_network_quality_scrape_success"].Metric {
				for _, l := range m.Label {
					if l.GetName() == "source" && l.GetValue() != "inventory" && m.Gauge.GetValue() != boolFloat64(!failure) {
						t.Error("incorrect API success")
					}
				}
			}
		})
	}
}

func TestNetworkQualityPartialFailureKeepsHealthySources(t *testing.T) {
	c := qualityTestCollector()
	c.getAPUplink = func(string) (*model.APUplinkStats, error) { return nil, errors.New("API unsupported") }
	f := gatherQuality(t, c)
	if f["omada_gateway_wan_quality_mos_average"] == nil || f["omada_ap_uplink_packets_total"] != nil {
		t.Fatal("partial failure lost healthy source or fabricated AP data")
	}
}

func TestNetworkQualitySkipsOfflineAndWirelessUplinks(t *testing.T) {
	c := qualityTestCollector()
	c.getDevices = func() ([]model.DevicesInterface, error) {
		return []model.DevicesInterface{
			&model.AccessPoint{Device: model.Device{Mac: "offline", Type: "ap", Status: 0}},
			&model.AccessPoint{Device: model.Device{Mac: "mesh", Type: "ap", Status: 15}, WirelessLinked: true},
			&model.AccessPoint{Device: model.Device{Mac: "mesh-status-only", Type: "ap", Status: 15}},
			&model.AccessPoint{Device: model.Device{Mac: "mesh-migrating", Type: "ap", Status: 17}},
			&model.Gateway{Device: model.Device{Mac: "offlinegw", Type: "gateway", Status: 0}},
		}, nil
	}
	c.getAPUplink = func(string) (*model.APUplinkStats, error) { t.Error("unexpected AP call"); return nil, nil }
	c.getWANQuality = func(string, int) (*model.WANQuality, error) { t.Error("unexpected WAN call"); return nil, nil }
	f := gatherQuality(t, c)
	if len(f["omada_network_quality_available"].Metric) != 5 || len(f["omada_network_quality_scrape_success"].Metric) != 1 {
		t.Fatal("bad skipped-device status")
	}
}

func TestNetworkQualityDisabledAndInventoryFailure(t *testing.T) {
	c := qualityTestCollector()
	c.client.Settings().TrackNetworkQualityMetrics = false
	c.getDevices = func() ([]model.DevicesInterface, error) { t.Error("disabled collector made request"); return nil, nil }
	if len(gatherQuality(t, c)) != 0 {
		t.Fatal("disabled collector emitted data")
	}
	c.client.Settings().TrackNetworkQualityMetrics = true
	c.getDevices = func() ([]model.DevicesInterface, error) { return nil, errors.New("inventory failed") }
	f := gatherQuality(t, c)
	if len(f) != 1 || f["omada_network_quality_scrape_success"].Metric[0].Gauge.GetValue() != 0 {
		t.Fatal("inventory failure not reported")
	}
}

func TestNetworkQualityFiltersInvalidReadingsAndUsesDefaultWindow(t *testing.T) {
	c := qualityTestCollector()
	c.client.Settings().NetworkQualityWindowSeconds = 0
	c.getWANQuality = func(_ string, window int) (*model.WANQuality, error) {
		if window != 86400 {
			t.Errorf("window=%d", window)
		}
		return &model.WANQuality{LatencyMilliseconds: new(-1.0), JitterMilliseconds: new(math.NaN()), PacketLossPercent: new(101.0), MOS: new(6.0)}, nil
	}
	f := gatherQuality(t, c)
	for name := range f {
		if strings.HasPrefix(name, "omada_gateway_wan_quality_") {
			t.Errorf("invalid metric %s exported", name)
		}
	}
}

func TestAPRadioInterferencePresenceAndBands(t *testing.T) {
	var ap model.AccessPoint
	if err := json.Unmarshal([]byte(`{"type":"ap","mac":"ap","name":"AP","status":15,"wp2g":{"interUtil":4},"wp5g":{"interUtil":0},"wp5g2":{"interUtil":2},"wp6g":{"interUtil":1}}`), &ap); err != nil {
		t.Fatal(err)
	}
	c := NewDeviceCollector(omada.NewClient(&session.Client{Config: &config.Config{Site: "test"}}))
	metrics := make(chan prometheus.Metric, 32)
	if err := c.collectAccessPoint(metrics, &ap, "site-id"); err != nil {
		t.Fatal(err)
	}
	close(metrics)
	got := map[string]float64{}
	for metric := range metrics {
		if !strings.Contains(metric.Desc().String(), "omada_ap_interference_percentage") {
			continue
		}
		m := &dto.Metric{}
		if err := metric.Write(m); err != nil {
			t.Fatal(err)
		}
		for _, label := range m.Label {
			if label.GetName() == "radio_band" {
				got[label.GetValue()] = m.Gauge.GetValue()
			}
		}
	}
	for band, want := range map[string]float64{"2.4": 4, "5": 0, "5-2": 2, "6": 1} {
		if v, ok := got[band]; !ok || v != want {
			t.Errorf("band %s=%v/%v", band, v, ok)
		}
	}
}

func TestAPRadioInterferenceOmitsMissingInvalidAndOffline(t *testing.T) {
	for _, payload := range []string{
		`{"status":14,"wp2g":{}}`, `{"status":14,"wp2g":{"interUtil":null}}`,
		`{"status":14,"wp2g":{"interUtil":-1}}`, `{"status":14,"wp2g":{"interUtil":101}}`,
		`{"status":0,"wp2g":{"interUtil":4}}`,
	} {
		var ap model.AccessPoint
		if err := json.Unmarshal([]byte(payload), &ap); err != nil {
			t.Fatal(err)
		}
		c := NewDeviceCollector(omada.NewClient(&session.Client{Config: &config.Config{}}))
		ch := make(chan prometheus.Metric, 32)
		_ = c.collectAccessPoint(ch, &ap, "site-id")
		close(ch)
		for metric := range ch {
			if strings.Contains(metric.Desc().String(), "omada_ap_interference_percentage") {
				t.Errorf("invalid interference emitted for %s", payload)
			}
		}
	}
}
