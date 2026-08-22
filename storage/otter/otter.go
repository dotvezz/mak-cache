package otter

import (
	"context"
	"fmt"
	"time"

	"github.com/dotvezz/mak-cache/config"
	"github.com/dotvezz/mak-cache/storage"
	"github.com/maypok86/otter/v2"
)

var now = time.Now

type Provider[T storage.Storable] struct {
	cache *otter.Cache[string, T]
	now   func() time.Time
}

func (p Provider[T]) Get(_ context.Context, key string) (T, error) {
	r, ok := p.cache.GetEntry(key)
	if !ok {
		return r.Value, storage.ErrNotFound
	}

	return r.Value, nil
}

func (p Provider[T]) Set(_ context.Context, key string, value T) error {
	value.RefreshHeapSize()
	_, _ = p.cache.Set(key, value)
	return nil
}

func (p Provider[T]) Update(_ context.Context, k string, value T) error {
	return p.Set(nil, k, value)
}

func NewProvider[T storage.Storable](cfg config.OtterConfig) (*Provider[T], error) {
	c, err := otter.New(&otter.Options[string, T]{
		MaximumWeight:   cfg.MemoryLimit,
		InitialCapacity: 1000,
		Weigher: func(key string, value T) uint32 {
			// plus 8 bytes for the pointer to the value
			return uint32(len(key)+value.HeapSize()) + 8
		},
		ExpiryCalculator: otter.ExpiryWritingFunc[string, T](func(entry otter.Entry[string, T]) time.Duration {
			return entry.Value.EvictAt().Sub(now())
		}),
	})

	if err != nil {
		return nil, fmt.Errorf("error creating otter instance: %w", err)
	}

	return &Provider[T]{
		cache: c,
		now:   now,
	}, nil
}
