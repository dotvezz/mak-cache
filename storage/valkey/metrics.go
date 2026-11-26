package valkey

import (
	"fmt"

	"github.com/dotvezz/mak-cache/cache"

	"github.com/prometheus/client_golang/prometheus"
)

// ProvisionMetrics implements metrics.Provisioner, enabling metrics exporting from the Valkey/Redis
// storage provider.
func (p *Provider[C, T]) ProvisionMetrics(prefix string, reg prometheus.Registerer) (err error) {
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
		Name: fmt.Sprintf("%s_redis_valkey_hits", prefix),
		Help: fmt.Sprintf("Number of%s hits from the Valkey/Redis storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.hits)
	if err != nil {
		return err
	}

	p.gets = prometheus.NewCounter(prometheus.CounterOpts{
		Name: fmt.Sprintf("%s_redis_valkey_gets", prefix),
		Help: fmt.Sprintf("Number of%s get operations from the Valkey/Redis storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.gets)
	if err != nil {
		return err
	}

	p.sets = prometheus.NewCounter(prometheus.CounterOpts{
		Name: fmt.Sprintf("%s_redis_valkey_sets", prefix),
		Help: fmt.Sprintf("Number of%s set operations from the Valkey/Redis storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.sets)
	if err != nil {
		return err
	}

	p.getErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: fmt.Sprintf("%s_redis_valkey_get_errors", prefix),
		Help: fmt.Sprintf("Number of%s get errors from the Valkey/Redis storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.getErrors)
	if err != nil {
		return err
	}

	p.setErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: fmt.Sprintf("%s_redis_valkey_set_errors", prefix),
		Help: fmt.Sprintf("Number of%s set errors from the Valkey/Redis storage for %s", helpInsert, prefix),
	})
	err = reg.Register(p.setErrors)
	if err != nil {
		return err
	}

	// Timing buckets for valkey start at 1 millisecond, as opposed to Otter where they start at 1 nanosecond,
	// to account for the difference in latency implications of each system.
	p.getSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                        fmt.Sprintf("%s_redis_valkey_get_seconds", prefix),
		Help:                        fmt.Sprintf("Get%s time in seconds from the Valkey/Redis storage for %s", helpInsert, prefix),
		Buckets:                     prometheus.ExponentialBuckets(0.001, 2, 8),
		NativeHistogramBucketFactor: 1.1,
	})
	err = reg.Register(p.getSeconds)
	if err != nil {
		return err
	}

	p.setSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                        fmt.Sprintf("%s_redis_valkey_set_seconds", prefix),
		Help:                        fmt.Sprintf("Set%s time in seconds from the Valkey/Redis storage for %s", helpInsert, prefix),
		Buckets:                     prometheus.ExponentialBuckets(0.001, 2, 8),
		NativeHistogramBucketFactor: 1.1,
	})
	err = reg.Register(p.setSeconds)
	if err != nil {
		return err
	}

	return nil
}
