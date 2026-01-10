package collector

import (
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/prometheus/client_golang/prometheus"
)

func (c *deviceCollector) collectOlt(ch chan<- prometheus.Metric, olt *model.Olt) error {

	return nil
}
