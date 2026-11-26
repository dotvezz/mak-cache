package otter

import (
	"fmt"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/prometheus/client_golang/prometheus"
)

// ProvisionMetrics implements metrics.Provisioner, enabling metrics exporting for the Otter in-memory
// storage provider.
func (p *Provider[T]) ProvisionMetrics(prefix string, reg prometheus.Registerer) (err error) {
	var helpInsert string
	{
		var t T
		switch any(t).(type) {
		case *cache.Metadata:
			prefix = prefix + "_metadata"
			helpInsert = " Metadata"
		case *cache.Entry:
			prefix = prefix + "_entry"
			helpInsert = " Entry"
		}
	}

	p.hits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: fmt.Sprintf("%s_otter_in_memory_hits", prefix),
		Help: fmt.Sprintf("Number of%s hits from the Otter in-memory storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.hits)
	if err != nil {
		return err
	}

	p.gets = prometheus.NewCounter(prometheus.CounterOpts{
		Name: fmt.Sprintf("%s_otter_in_memory_gets", prefix),
		Help: fmt.Sprintf("Number of%s get operations from the Otter in-memory storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.gets)
	if err != nil {
		return err
	}

	p.sets = prometheus.NewCounter(prometheus.CounterOpts{
		Name: fmt.Sprintf("%s_otter_in_memory_sets", prefix),
		Help: fmt.Sprintf("Number of%s set operations to the Otter in-memory storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.sets)
	if err != nil {
		return err
	}

	p.getSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                        fmt.Sprintf("%s_otter_in_memory_get_seconds", prefix),
		Help:                        fmt.Sprintf("Get%s time in seconds from the Otter in-memory storage for %s", helpInsert, prefix),
		Buckets:                     prometheus.ExponentialBuckets(0.000000001, 4, 12),
		NativeHistogramBucketFactor: 1.1,
	})
	err = reg.Register(p.getSeconds)
	if err != nil {
		return err
	}

	p.setSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                        fmt.Sprintf("%s_otter_in_memory_set_seconds", prefix),
		Help:                        fmt.Sprintf("Set%s time in seconds from the Otter in-memory storage for %s", helpInsert, prefix),
		Buckets:                     prometheus.ExponentialBuckets(0.000000001, 4, 12),
		NativeHistogramBucketFactor: 1.1,
	})
	err = reg.Register(p.setSeconds)
	if err != nil {
		return err
	}

	return nil
}
