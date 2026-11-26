package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Provisioner interface {
	ProvisionMetrics(prefix string, reg prometheus.Registerer) error
}
