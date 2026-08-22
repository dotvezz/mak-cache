package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/storage"
)

func (m *Middleware) getMetadata(ctx context.Context, key string) (md *cache.Metadata, found bool) {
	key = "m" + key
	var err error
	if m.metadataStorage == nil {
		// Allow for entry storage to act as metadata storage, if no dedicated metadata storage is configured
		var e *cache.Entry
		e, err = m.entryStorage.Get(ctx, key)
		if err == nil {
			md = &e.Metadata
		}
	} else {
		md, err = m.metadataStorage.Get(ctx, key)
	}

	found = err == nil
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		m.Error("getMetadata",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)
	}

	return md, found
}

func (m *Middleware) setMetadata(ctx context.Context, key string, meta *cache.Metadata) error {
	if meta == nil {
		return fmt.Errorf("metadata is nil")
	}
	key = "m" + key

	// Allow for entry storage to act as metadata storage, if no dedicated metadata storage is configured
	if m.metadataStorage == nil {
		e := &cache.Entry{
			Metadata: *meta,
		}
		return m.entryStorage.Set(ctx, key, e)
	}

	return m.metadataStorage.Set(ctx, key, meta)
}

func (m *Middleware) updateMetadata(ctx context.Context, key string, md *cache.Metadata) error {
	if md == nil {
		return fmt.Errorf("metadata is nil")
	}
	key = "m" + key

	// Allow for entry storage to act as metadata storage, if no dedicated metadata storage is configured
	if m.metadataStorage == nil {
		e := &cache.Entry{
			Metadata: *md,
		}
		return m.entryStorage.Update(ctx, key, e)
	}

	return m.metadataStorage.Update(ctx, key, md)
}

func (m *Middleware) getEntry(ctx context.Context, key string) (e *cache.Entry, found bool) {
	key = "e" + key
	e, err := m.entryStorage.Get(ctx, key)
	found = err == nil
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		m.Error("getEntry",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)
	}

	return e, found
}

func (m *Middleware) setEntry(ctx context.Context, key string, entry *cache.Entry) error {
	key = "e" + key
	return m.entryStorage.Set(ctx, key, entry)
}

func (m *Middleware) updateEntry(ctx context.Context, key string, entry *cache.Entry) error {
	key = "e" + key
	return m.entryStorage.Update(ctx, key, entry)
}
