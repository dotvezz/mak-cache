package storage_test

import (
	"errors"
	"testing"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/metrics"
	"github.com/dotvezz/mak-cache/storage"
	"github.com/prometheus/client_golang/prometheus"
)

type mockProvisionableProvider struct {
	mockProvider
	provisionFunc func(prefix string, reg prometheus.Registerer) error
}

func (m *mockProvisionableProvider) ProvisionMetrics(prefix string, reg prometheus.Registerer) error {
	if m.provisionFunc != nil {
		return m.provisionFunc(prefix, reg)
	}
	return nil
}

var _ metrics.Provisioner = (*storage.Wrapper[*cache.Entry])(nil)

func TestWrap_ProvisionMetrics(t *testing.T) {
	t.Run("provisions both outer and inner", func(t *testing.T) {
		var order []string
		var gotOuterPrefix, gotInnerPrefix string
		reg := prometheus.NewRegistry()

		inner := &mockProvisionableProvider{
			provisionFunc: func(prefix string, r prometheus.Registerer) error {
				order = append(order, "inner")
				gotInnerPrefix = prefix
				if r != reg {
					t.Error("inner received unexpected registerer")
				}
				return nil
			},
		}
		outer := &mockProvisionableProvider{
			provisionFunc: func(prefix string, r prometheus.Registerer) error {
				order = append(order, "outer")
				gotOuterPrefix = prefix
				if r != reg {
					t.Error("outer received unexpected registerer")
				}
				return nil
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		prov, ok := wrapped.(metrics.Provisioner)
		if !ok {
			t.Fatal("expected wrapped storage to implement metrics.Provisioner")
		}

		if err := prov.ProvisionMetrics("my_prefix", reg); err != nil {
			t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
		}

		if gotOuterPrefix != "my_prefix" || gotInnerPrefix != "my_prefix" {
			t.Errorf("prefixes = (outer=%q, inner=%q), want both %q", gotOuterPrefix, gotInnerPrefix, "my_prefix")
		}
		if len(order) != 2 || order[0] != "outer" || order[1] != "inner" {
			t.Errorf("provision order = %v, want [outer inner]", order)
		}
	})

	t.Run("neither implements Provisioner", func(t *testing.T) {
		wrapped := storage.Wrap[*cache.Entry](&mockProvider{}, &mockProvider{})
		prov := wrapped.(metrics.Provisioner)
		if err := prov.ProvisionMetrics("p", prometheus.NewRegistry()); err != nil {
			t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
		}
	})

	t.Run("only inner implements Provisioner", func(t *testing.T) {
		called := false
		inner := &mockProvisionableProvider{
			provisionFunc: func(prefix string, r prometheus.Registerer) error {
				called = true
				return nil
			},
		}
		wrapped := storage.Wrap[*cache.Entry](inner, &mockProvider{})
		prov := wrapped.(metrics.Provisioner)
		if err := prov.ProvisionMetrics("p", prometheus.NewRegistry()); err != nil {
			t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
		}
		if !called {
			t.Error("expected inner ProvisionMetrics to be called")
		}
	})

	t.Run("outer error short-circuits inner", func(t *testing.T) {
		outerErr := errors.New("outer registration failed")
		innerCalled := false
		inner := &mockProvisionableProvider{
			provisionFunc: func(prefix string, r prometheus.Registerer) error {
				innerCalled = true
				return nil
			},
		}
		outer := &mockProvisionableProvider{
			provisionFunc: func(prefix string, r prometheus.Registerer) error {
				return outerErr
			},
		}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		prov := wrapped.(metrics.Provisioner)
		err := prov.ProvisionMetrics("p", prometheus.NewRegistry())
		if !errors.Is(err, outerErr) {
			t.Errorf("error = %v, want %v", err, outerErr)
		}
		if innerCalled {
			t.Error("expected inner ProvisionMetrics not to be called after outer error")
		}
	})

	t.Run("inner error is returned", func(t *testing.T) {
		innerErr := errors.New("inner registration failed")
		inner := &mockProvisionableProvider{
			provisionFunc: func(prefix string, r prometheus.Registerer) error {
				return innerErr
			},
		}
		outer := &mockProvisionableProvider{}

		wrapped := storage.Wrap[*cache.Entry](inner, outer)
		prov := wrapped.(metrics.Provisioner)
		err := prov.ProvisionMetrics("p", prometheus.NewRegistry())
		if !errors.Is(err, innerErr) {
			t.Errorf("error = %v, want %v", err, innerErr)
		}
	})

	t.Run("nested wrappers provision all layers", func(t *testing.T) {
		count := 0
		makeProv := func() *mockProvisionableProvider {
			return &mockProvisionableProvider{
				provisionFunc: func(prefix string, r prometheus.Registerer) error {
					count++
					return nil
				},
			}
		}

		layer1 := storage.Wrap[*cache.Entry](makeProv(), makeProv())
		layer2 := storage.Wrap[*cache.Entry](layer1, makeProv())

		prov := layer2.(metrics.Provisioner)
		if err := prov.ProvisionMetrics("nested", prometheus.NewRegistry()); err != nil {
			t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
		}
		if count != 3 {
			t.Errorf("provisioned %d layers, want 3", count)
		}
	})
}
