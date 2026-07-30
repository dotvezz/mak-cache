package cache

import (
	"context"
	"fmt"

	"github.com/dotvezz/caddy-cache/cache"
)

func (h *Handler) getMetadata(ctx context.Context, key string) (*cache.Metadata, error) {
	// Allow for entry storage to act as metadata storage, if no dedicated metadata storage is configured
	if h.metadataStorage == nil {
		e, err := h.entryStorage.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		return &e.Metadata, err
	}

	return h.metadataStorage.Get(ctx, key)
}

func (h *Handler) setMetadata(ctx context.Context, key string, meta *cache.Metadata) error {
	if meta == nil {
		return fmt.Errorf("metadata is nil")
	}

	// Allow for entry storage to act as metadata storage, if no dedicated metadata storage is configured
	if h.metadataStorage == nil {
		e := &cache.Entry{
			Metadata: *meta,
		}
		return h.entryStorage.Set(ctx, key, e)
	}

	return h.metadataStorage.Set(ctx, key, meta)
}
