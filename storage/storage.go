package storage

import (
	"context"

	"github.com/dotvezz/caddy-cache/cache"
)

type Storable interface {
	HeapSize() int
	RefreshHeapSize()
	MarshalTo(cache.Writer) error
	Unmarshal(cache.Reader) error
}

type Provider[T Storable] interface {
	Get(ctx context.Context, k string) (v T, err error)
	Set(ctx context.Context, k string, v T) error
	Update(ctx context.Context, k string, v T) error
}
