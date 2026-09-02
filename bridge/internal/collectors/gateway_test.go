package collector

import (
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestCollectGatewayExportsTemperatureAsGauge(t *testing.T) {
	apiClient := &api.Client{Config: &config.Config{Site: "test"}}
	collector := NewDeviceCollector(apiClient)
	gateway := &model.Gateway{Device: model.Device{Temp: 42}}
	metrics := make(chan prometheus.Metric, 3)

	if err := collector.collectGateway(metrics, gateway, "site-id"); err != nil {
		t.Fatal(err)
	}

	temperature := <-metrics
	metric := &dto.Metric{}
	if err := temperature.Write(metric); err != nil {
		t.Fatal(err)
	}
	if metric.Gauge == nil {
		t.Fatalf("temperature metric type = %T, want gauge", temperature)
	}
	if got := metric.Gauge.GetValue(); got != 42 {
		t.Fatalf("temperature = %v, want 42", got)
	}
}
