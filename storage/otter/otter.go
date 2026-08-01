package otter

import (
	"context"
	"fmt"
	"time"

	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/storage"
	"github.com/maypok86/otter/v2"
)

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
	_, _ = p.cache.Set(key, value)
	return nil
}

// Update is a noop for the otter provider. With Otter, T lives in heap and is mutable, and caddy-cache operates
// directly with values from Get, including for setting refreshed expiry etc. So actually invoking p.cache.Set in Update
// would be redundant.
func (p Provider[T]) Update(_ context.Context, _ string, _ T) error {
	return nil
}

func NewProvider[T storage.Storable](cfg config.OtterConfig) (*Provider[T], error) {
	c, err := otter.New(&otter.Options[string, T]{
		MaximumWeight:   cfg.MemoryLimit,
		InitialCapacity: 1000,
		Weigher: func(key string, value T) uint32 {
			// plus 8 bytes for the pointer to the value
			return uint32(len(key)+value.Size()) + 8
		},
	})

	if err != nil {
		return nil, fmt.Errorf("error creating otter instance: %w", err)
	}

	return &Provider[T]{
		cache: c,
		now:   time.Now,
	}, nil
}
