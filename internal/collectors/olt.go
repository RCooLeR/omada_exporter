package collector

import (
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/prometheus/client_golang/prometheus"
)

func (c *DeviceCollector) collectOlt(ch chan<- prometheus.Metric, olt *model.Olt) error {

	return nil
}
