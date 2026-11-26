package otter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/config"
	"github.com/dotvezz/mak-cache/metrics"
	"github.com/dotvezz/mak-cache/storage"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func counterVal(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("failed to read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

var (
	_ metrics.Provisioner = (*Provider[*cache.Entry])(nil)
	_ metrics.Provisioner = (*Provider[*cache.Metadata])(nil)
)

func gatherMap(t *testing.T, reg prometheus.Gatherer) map[string]*dto.MetricFamily {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	res := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		res[mf.GetName()] = mf
	}
	return res
}

func TestProvider_ProvisionMetrics_Entry(t *testing.T) {
	p, err := NewProvider[*cache.Entry](config.OtterConfig{MemoryLimit: 10_000_000})
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}

	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("mak", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	got := gatherMap(t, reg)
	want := map[string]dto.MetricType{
		"mak_entry_otter_in_memory_hits":        dto.MetricType_COUNTER,
		"mak_entry_otter_in_memory_gets":        dto.MetricType_COUNTER,
		"mak_entry_otter_in_memory_sets":        dto.MetricType_COUNTER,
		"mak_entry_otter_in_memory_get_seconds": dto.MetricType_HISTOGRAM,
		"mak_entry_otter_in_memory_set_seconds": dto.MetricType_HISTOGRAM,
	}

	if len(got) != len(want) {
		t.Errorf("registered %d metric families, want %d", len(got), len(want))
	}

	for name, wantType := range want {
		mf, ok := got[name]
		if !ok {
			t.Errorf("expected metric %q to be registered", name)
			continue
		}
		if mf.GetType() != wantType {
			t.Errorf("metric %q type = %v, want %v", name, mf.GetType(), wantType)
		}
		if !strings.Contains(mf.GetHelp(), "Entry") {
			t.Errorf("metric %q help %q expected to contain 'Entry'", name, mf.GetHelp())
		}
	}
}

func TestProvider_ProvisionMetrics_Metadata(t *testing.T) {
	p, err := NewProvider[*cache.Metadata](config.OtterConfig{MemoryLimit: 10_000_000})
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}

	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("mak", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	got := gatherMap(t, reg)
	want := map[string]dto.MetricType{
		"mak_metadata_otter_in_memory_hits":        dto.MetricType_COUNTER,
		"mak_metadata_otter_in_memory_gets":        dto.MetricType_COUNTER,
		"mak_metadata_otter_in_memory_sets":        dto.MetricType_COUNTER,
		"mak_metadata_otter_in_memory_get_seconds": dto.MetricType_HISTOGRAM,
		"mak_metadata_otter_in_memory_set_seconds": dto.MetricType_HISTOGRAM,
	}

	if len(got) != len(want) {
		t.Errorf("registered %d metric families, want %d", len(got), len(want))
	}

	for name, wantType := range want {
		mf, ok := got[name]
		if !ok {
			t.Errorf("expected metric %q to be registered", name)
			continue
		}
		if mf.GetType() != wantType {
			t.Errorf("metric %q type = %v, want %v", name, mf.GetType(), wantType)
		}
		if !strings.Contains(mf.GetHelp(), "Metadata") {
			t.Errorf("metric %q help %q expected to contain 'Metadata'", name, mf.GetHelp())
		}
	}
}

func TestProvider_ProvisionMetrics_DuplicateRegistrationError(t *testing.T) {
	p1, err := NewProvider[*cache.Entry](config.OtterConfig{MemoryLimit: 10_000_000})
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}
	p2, err := NewProvider[*cache.Entry](config.OtterConfig{MemoryLimit: 10_000_000})
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}

	reg := prometheus.NewRegistry()
	if err := p1.ProvisionMetrics("dup", reg); err != nil {
		t.Fatalf("first ProvisionMetrics() unexpected error: %v", err)
	}
	if err := p2.ProvisionMetrics("dup", reg); err == nil {
		t.Fatal("second ProvisionMetrics() expected error, got nil")
	}
}

func TestProvider_Metrics_GetHitMissSetUpdate(t *testing.T) {
	p, err := NewProvider[*cache.Entry](config.OtterConfig{MemoryLimit: 10_000_000})
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}

	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("test", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	ctx := context.Background()

	// 1. Miss
	_, err = p.Get(ctx, "missing")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrNotFound", err)
	}
	if v := counterVal(t, p.gets); v != 1 {
		t.Errorf("gets after miss = %v, want 1", v)
	}
	if v := counterVal(t, p.hits); v != 0 {
		t.Errorf("hits after miss = %v, want 0", v)
	}

	// 2. Set
	entry := &cache.Entry{
		Status: 200,
		Body:   []byte("hello"),
		Metadata: cache.Metadata{
			Expires: time.Now().Add(time.Minute),
			Evict:   time.Now().Add(2 * time.Minute),
		},
	}
	if err := p.Set(ctx, "k1", entry); err != nil {
		t.Fatalf("Set(k1) unexpected error: %v", err)
	}
	if v := counterVal(t, p.sets); v != 1 {
		t.Errorf("sets after Set = %v, want 1", v)
	}

	// 3. Hit
	got, err := p.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get(k1) unexpected error: %v", err)
	}
	if string(got.Body) != "hello" {
		t.Errorf("Get(k1) body = %q, want %q", string(got.Body), "hello")
	}
	if v := counterVal(t, p.gets); v != 2 {
		t.Errorf("gets after hit = %v, want 2", v)
	}
	if v := counterVal(t, p.hits); v != 1 {
		t.Errorf("hits after hit = %v, want 1", v)
	}

	// 4. Update
	entry2 := &cache.Entry{
		Status: 200,
		Body:   []byte("updated"),
		Metadata: cache.Metadata{
			Expires: time.Now().Add(time.Minute),
			Evict:   time.Now().Add(2 * time.Minute),
		},
	}
	if err := p.Update(ctx, "k1", entry2); err != nil {
		t.Fatalf("Update(k1) unexpected error: %v", err)
	}
	if v := counterVal(t, p.sets); v != 2 {
		t.Errorf("sets after Update = %v, want 2", v)
	}

	// Verify histogram sample counts
	mfs := gatherMap(t, reg)
	getHist := mfs["test_entry_otter_in_memory_get_seconds"].GetMetric()[0].GetHistogram()
	if getHist.GetSampleCount() != 2 {
		t.Errorf("get_seconds sample count = %d, want 2", getHist.GetSampleCount())
	}
	setHist := mfs["test_entry_otter_in_memory_set_seconds"].GetMetric()[0].GetHistogram()
	if setHist.GetSampleCount() != 2 {
		t.Errorf("set_seconds sample count = %d, want 2", setHist.GetSampleCount())
	}
}

func TestProvider_Metrics_TimingObservation(t *testing.T) {
	p, err := NewProvider[*cache.Entry](config.OtterConfig{MemoryLimit: 10_000_000})
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}

	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("timing", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	origNow := now
	now = func() time.Time {
		calls++
		// Each call advances by 5ms
		return base.Add(time.Duration(calls) * 5 * time.Millisecond)
	}
	defer func() { now = origNow }()

	_, _ = p.Get(context.Background(), "miss")

	mfs := gatherMap(t, reg)
	getHist := mfs["timing_entry_otter_in_memory_get_seconds"].GetMetric()[0].GetHistogram()
	if getHist.GetSampleCount() != 1 {
		t.Fatalf("get_seconds sample count = %d, want 1", getHist.GetSampleCount())
	}
	if getHist.GetSampleSum() < 0.004 || getHist.GetSampleSum() > 0.006 {
		t.Errorf("get_seconds sample sum = %v, want ~0.005s", getHist.GetSampleSum())
	}
}

func TestProvider_Metrics_WithoutProvisioning(t *testing.T) {
	p, err := NewProvider[*cache.Entry](config.OtterConfig{MemoryLimit: 10_000_000})
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}

	ctx := context.Background()
	entry := &cache.Entry{
		Status: 200,
		Body:   []byte("no-metrics"),
		Metadata: cache.Metadata{
			Expires: time.Now().Add(time.Minute),
			Evict:   time.Now().Add(2 * time.Minute),
		},
	}

	// None of these should panic when ProvisionMetrics has not been called
	if err := p.Set(ctx, "k", entry); err != nil {
		t.Fatalf("Set() unexpected error: %v", err)
	}
	if _, err := p.Get(ctx, "k"); err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if err := p.Update(ctx, "k", entry); err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
}
