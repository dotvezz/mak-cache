package storage

import (
	"context"
	"errors"
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

func (w Wrapper[T]) Get(ctx context.Context, k string) (v T, err error) {
	v, err = w.outer.Get(ctx, k)

	if err != nil && errors.Is(err, ErrNotFound) {
		v, err = w.inner.Get(ctx, k)
		if err != nil {
			// Since we found it on the outer, silently try to set it on the outer to bring the entry into the
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
	if err1 != nil && err2 != nil {
		return errors.Join(err1, err2)
	}

	return nil
}
