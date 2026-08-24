package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/storage"
)

type mockProvider struct {
	getFunc    func(ctx context.Context, key string) (*cache.Entry, error)
	setFunc    func(ctx context.Context, key string, val *cache.Entry) error
	updateFunc func(ctx context.Context, key string, val *cache.Entry) error
}

func (m *mockProvider) Get(ctx context.Context, key string) (*cache.Entry, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, key)
	}
	return nil, storage.ErrNotFound
}

func (m *mockProvider) Set(ctx context.Context, key string, val *cache.Entry) error {
	if m.setFunc != nil {
		return m.setFunc(ctx, key, val)
	}
	return nil
}

func (m *mockProvider) Update(ctx context.Context, key string, val *cache.Entry) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, key, val)
	}
	return nil
}

func TestWrap_Get(t *testing.T) {
	t.Run("outer hit", func(t *testing.T) {
		inner := &mockProvider{
			getFunc: func(ctx context.Context, key string) (*cache.Entry, error) {
				t.Fatal("inner should not be called when outer hits")
				return nil, nil
			},
		}
		outer := &mockProvider{
			getFunc: func(ctx context.Context, key string) (*cache.Entry, error) {
				return &cache.Entry{Body: []byte("outer")}, nil
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		val, err := wrapped.Get(context.Background(), "k1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(val.Body) != "outer" {
			t.Errorf("got %q, want %q", string(val.Body), "outer")
		}
	})

	t.Run("outer miss, inner hit populates outer", func(t *testing.T) {
		outerSet := false
		outer := &mockProvider{
			getFunc: func(ctx context.Context, key string) (*cache.Entry, error) {
				return nil, storage.ErrNotFound
			},
			setFunc: func(ctx context.Context, key string, val *cache.Entry) error {
				outerSet = true
				if string(val.Body) != "inner" {
					t.Errorf("set got %q, want %q", string(val.Body), "inner")
				}
				return nil
			},
		}
		inner := &mockProvider{
			getFunc: func(ctx context.Context, key string) (*cache.Entry, error) {
				return &cache.Entry{Body: []byte("inner")}, nil
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		val, err := wrapped.Get(context.Background(), "k1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(val.Body) != "inner" {
			t.Errorf("got %q, want %q", string(val.Body), "inner")
		}
		if !outerSet {
			t.Error("expected inner hit to populate outer storage")
		}
	})

	t.Run("both miss", func(t *testing.T) {
		inner := &mockProvider{
			getFunc: func(ctx context.Context, key string) (*cache.Entry, error) {
				return nil, storage.ErrNotFound
			},
		}
		outer := &mockProvider{
			getFunc: func(ctx context.Context, key string) (*cache.Entry, error) {
				return nil, storage.ErrNotFound
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		_, err := wrapped.Get(context.Background(), "k1")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
		}
	})

	t.Run("outer non-notfound error returns error", func(t *testing.T) {
		customErr := errors.New("db error")
		outer := &mockProvider{
			getFunc: func(ctx context.Context, key string) (*cache.Entry, error) {
				return nil, customErr
			},
		}
		inner := &mockProvider{}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		_, err := wrapped.Get(context.Background(), "k1")
		if !errors.Is(err, customErr) {
			t.Errorf("got error %v, want %v", err, customErr)
		}
	})
}

func TestWrap_Set(t *testing.T) {
	t.Run("sets both inner and outer", func(t *testing.T) {
		innerSet, outerSet := false, false
		inner := &mockProvider{
			setFunc: func(ctx context.Context, key string, val *cache.Entry) error {
				innerSet = true
				return nil
			},
		}
		outer := &mockProvider{
			setFunc: func(ctx context.Context, key string, val *cache.Entry) error {
				outerSet = true
				return nil
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		err := wrapped.Set(context.Background(), "k1", &cache.Entry{Body: []byte("v1")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !innerSet || !outerSet {
			t.Errorf("expected both to be set, innerSet=%v, outerSet=%v", innerSet, outerSet)
		}
	})

	t.Run("returns error when both inner and outer fail", func(t *testing.T) {
		err1 := errors.New("inner error")
		err2 := errors.New("outer error")
		inner := &mockProvider{
			setFunc: func(ctx context.Context, key string, val *cache.Entry) error {
				return err1
			},
		}
		outer := &mockProvider{
			setFunc: func(ctx context.Context, key string, val *cache.Entry) error {
				return err2
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		err := wrapped.Set(context.Background(), "k1", &cache.Entry{Body: []byte("v1")})
		if err == nil {
			t.Error("expected combined error when both fail, got nil")
		}
	})
}

func TestWrap_Update(t *testing.T) {
	t.Run("updates both inner and outer", func(t *testing.T) {
		innerUpd, outerUpd := false, false
		inner := &mockProvider{
			updateFunc: func(ctx context.Context, key string, val *cache.Entry) error {
				innerUpd = true
				return nil
			},
		}
		outer := &mockProvider{
			updateFunc: func(ctx context.Context, key string, val *cache.Entry) error {
				outerUpd = true
				return nil
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		err := wrapped.Update(context.Background(), "k1", &cache.Entry{Body: []byte("v1")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !innerUpd || !outerUpd {
			t.Errorf("expected both to be updated, innerUpd=%v, outerUpd=%v", innerUpd, outerUpd)
		}
	})
}
