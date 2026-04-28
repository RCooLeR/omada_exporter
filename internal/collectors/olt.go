package collector

import (
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/prometheus/client_golang/prometheus"
)

// collectOlt emits metrics for the OLT.
func (c *DeviceCollector) collectOlt(ch chan<- prometheus.Metric, olt *model.Olt) error {

	return nil
}
