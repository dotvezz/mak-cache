package otter

import (
	"context"
	"reflect"
	"testing"

	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/storage"
)

func TestNewProvider(t *testing.T) {
	t.Run("Valid configuration", func(t *testing.T) {
		cfg := config.OtterConfig{
			MemoryLimit: 10000,
		}

		provider, err := NewProvider[*cache.Metadata](cfg)
		if err != nil {
			t.Fatalf("expected no error creating provider, got: %v", err)
		}
		if provider == nil {
			t.Fatal("expected provider to be non-nil")
		}
	})

	t.Run("Invalid configuration", func(t *testing.T) {
		cfg := config.OtterConfig{
			MemoryLimit: 0,
		}

		provider, err := NewProvider[*cache.Metadata](cfg)
		if err == nil {
			t.Fatal("expected error creating provider with invalid memory limit, got nil")
		}
		if provider != nil {
			t.Errorf("expected provider to be nil on error, got %v", provider)
		}
	})
}

func TestProvider_Metadata_SetGet(t *testing.T) {
	type testCase[T storage.Storable] struct {
		name                       string
		key                        string
		value                      *cache.Metadata
		wantSetError, wantGetError bool
	}
	tests := []testCase[*cache.Metadata]{
		{
			name: "simple",
			key:  "simple",
			value: &cache.Metadata{
				ETag: "eTag",
			},
		},
	}
	p, _ := NewProvider[*cache.Metadata](config.OtterConfig{
		MemoryLimit: 10_000_000,
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Set(context.Background(), tt.key, tt.value)
			if err != nil && !tt.wantSetError {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantSetError)
				return
			}
			got, err := p.Get(context.Background(), tt.key)
			if err != nil != tt.wantGetError {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantGetError)
				return
			}
			if !reflect.DeepEqual(got, tt.value) {
				t.Errorf("Get() got = %v, want %v", got, tt.value)
			}
		})
	}
}
