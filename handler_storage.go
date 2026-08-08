package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/storage"
)

func (h *Handler) getMetadata(ctx context.Context, key string) (m *cache.Metadata, found bool) {
	var err error
	if h.metadataStorage == nil {
		// Allow for entry storage to act as metadata storage, if no dedicated metadata storage is configured
		var e *cache.Entry
		e, err = h.entryStorage.Get(ctx, key)
		if err == nil {
			m = &e.Metadata
		}
	} else {
		m, err = h.metadataStorage.Get(ctx, key)
	}

	found = err == nil
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		h.Error("getMetadata",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)
	}

	return m, found
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

func (h *Handler) updateMetadata(ctx context.Context, key string, meta *cache.Metadata) error {
	if meta == nil {
		return fmt.Errorf("metadata is nil")
	}

	// Allow for entry storage to act as metadata storage, if no dedicated metadata storage is configured
	if h.metadataStorage == nil {
		e := &cache.Entry{
			Metadata: *meta,
		}
		return h.entryStorage.Update(ctx, key, e)
	}

	return h.metadataStorage.Update(ctx, key, meta)
}

func (h *Handler) getEntry(ctx context.Context, key string) (e *cache.Entry, found bool) {
	e, err := h.entryStorage.Get(ctx, key)
	found = err == nil
	if err != nil {
		h.Error("getEntry",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)
	}

	return e, found
}

func (h *Handler) setEntry(ctx context.Context, key string, entry *cache.Entry) error {
	return h.entryStorage.Set(ctx, key, entry)
}

func (h *Handler) updateEntry(ctx context.Context, key string, entry *cache.Entry) error {
	return h.entryStorage.Update(ctx, key, entry)
}
