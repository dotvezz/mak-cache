package mak

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	requests        prometheus.Counter
	hits            prometheus.Counter
	stale           prometheus.Counter
	collapses       prometheus.Counter
	errors          prometheus.Counter
	ttfbSeconds     prometheus.Histogram
	transferSeconds prometheus.Histogram
}

func (m *metrics) ProvisionMetrics(prefix string, reg prometheus.Registerer) (err error) {
	m.hits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: prefix + "_hits",
		Help: "Number of cache hits",
	})
	err = reg.Register(m.hits)
	if err != nil {
		return err
	}

	m.requests = prometheus.NewCounter(prometheus.CounterOpts{
		Name: prefix + "_requests",
		Help: "Number of cache misses",
	})
	err = reg.Register(m.requests)
	if err != nil {
		return err
	}

	m.stale = prometheus.NewCounter(prometheus.CounterOpts{
		Name: prefix + "_stale_count",
		Help: "Number of stale cache entries",
	})
	err = reg.Register(m.stale)
	if err != nil {
		return err
	}

	m.collapses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: prefix + "_collapse_count",
		Help: "Number of cache collapses",
	})
	err = reg.Register(m.collapses)
	if err != nil {
		return err
	}

	m.errors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: prefix + "_error_count",
		Help: "Number of cache errors",
	})
	err = reg.Register(m.errors)
	if err != nil {
		return err
	}

	m.ttfbSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                        prefix + "_ttfb_seconds",
		Help:                        "Time to first byte",
		NativeHistogramBucketFactor: 1.1,
	})
	err = reg.Register(m.ttfbSeconds)
	if err != nil {
		return err
	}

	m.transferSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                        prefix + "_transfer_seconds",
		Help:                        "Time for full transfer",
		NativeHistogramBucketFactor: 1.1,
	})
	err = reg.Register(m.transferSeconds)
	if err != nil {
		return err
	}

	return nil
}
