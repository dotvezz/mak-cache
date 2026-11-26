package storage

import (
	"context"
	"errors"

	"github.com/dotvezz/mak-cache/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func Wrap[T Storable](inner, outer Provider[T]) Provider[T] {
	return &Wrapper[T]{
		inner,
		outer,
	}
}

type Wrapper[T Storable] struct {
	inner Provider[T]
	outer Provider[T]
}

func (w Wrapper[T]) ProvisionMetrics(prefix string, reg prometheus.Registerer) (err error) {
	if outerProvisioner, ok := w.outer.(metrics.Provisioner); ok {
		err = outerProvisioner.ProvisionMetrics(prefix, reg)
		if err != nil {
			return
		}
	}

	if innerProvisioner, ok := w.inner.(metrics.Provisioner); ok {
		err = innerProvisioner.ProvisionMetrics(prefix, reg)
		if err != nil {
			return
		}
	}

	return
}

func (w Wrapper[T]) Get(ctx context.Context, k string) (v T, err error) {
	v, err = w.outer.Get(ctx, k)

	if err != nil && errors.Is(err, ErrNotFound) {
		v, err = w.inner.Get(ctx, k)
		if err == nil {
			// Since we found it on the inner, silently try to set it on the outer to bring the entry into the
			// nearer layer.
			_ = w.outer.Set(ctx, k, v)
		}
	}

	return
}

func (w Wrapper[T]) Set(ctx context.Context, k string, v T) error {
	err1 := w.inner.Set(ctx, k, v)
	err2 := w.outer.Set(ctx, k, v)

	// TODO: Less stupid error handling here
	if err1 != nil || err2 != nil {
		return errors.Join(err1, err2)
	}

	return nil
}

func (w Wrapper[T]) Update(ctx context.Context, k string, v T) error {
	err1 := w.inner.Update(ctx, k, v)
	err2 := w.outer.Update(ctx, k, v)

	// TODO: Less stupid error handling here
	if err1 != nil || err2 != nil {
		return errors.Join(err1, err2)
	}

	return nil
}
