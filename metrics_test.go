package mak

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	mock_origin "github.com/dotvezz/mak-cache/_test/mock-origin"
	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/config"
	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"golang.org/x/sync/singleflight"
)

func gatherMetricsMap(t *testing.T, reg prometheus.Gatherer) map[string]*dto.MetricFamily {
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

func counterValue(t *testing.T, mfs map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	mf, ok := mfs[name]
	if !ok {
		t.Fatalf("metric %q not found in registry", name)
	}
	if len(mf.GetMetric()) == 0 {
		t.Fatalf("metric %q has no samples", name)
	}
	return mf.GetMetric()[0].GetCounter().GetValue()
}

func histogramCount(t *testing.T, mfs map[string]*dto.MetricFamily, name string) uint64 {
	t.Helper()
	mf, ok := mfs[name]
	if !ok {
		t.Fatalf("metric %q not found in registry", name)
	}
	if len(mf.GetMetric()) == 0 {
		t.Fatalf("metric %q has no samples", name)
	}
	return mf.GetMetric()[0].GetHistogram().GetSampleCount()
}

func newMetricsTestConfig(prefix string, withMetadataStorage bool) config.Config {
	cfg := config.Config{
		Timing: config.TimingConfig{
			TTL:      10 * time.Second,
			MaxStale: 30 * time.Second,
		},
		Prometheus: config.PrometheusConfig{
			Prefix: prefix,
		},
		Storage: []config.StorageConfig{
			{
				Otter: &config.OtterConfig{
					MemoryLimit: must(humanize.ParseBytes("100MB")),
				},
			},
		},
	}
	if withMetadataStorage {
		cfg.MetadataStorage = []config.StorageConfig{
			{
				Otter: &config.OtterConfig{
					MemoryLimit: must(humanize.ParseBytes("100MB")),
				},
			},
		}
	}
	return cfg
}

func TestMiddleware_Metrics_DisabledWhenNoRegistererOrNoPrefix(t *testing.T) {
	t.Run("no registerer with prefix set", func(t *testing.T) {
		mw, err := New(WithConfig(newMetricsTestConfig("mak_noreg", false)))
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}
		origin := new(mock_origin.MockOrigin)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/cacheable", nil)
		mw(origin).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("registerer provided with empty prefix registers nothing", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		mw, err := New(
			WithConfig(newMetricsTestConfig("", true)),
			WithPrometheusRegistrer(reg),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}
		mfs := gatherMetricsMap(t, reg)
		if len(mfs) != 0 {
			t.Errorf("expected 0 metrics registered when prefix is empty, got %d", len(mfs))
		}

		origin := new(mock_origin.MockOrigin)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/cacheable", nil)
		mw(origin).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}

func TestMiddleware_Metrics_Provisioning(t *testing.T) {
	reg := prometheus.NewRegistry()
	const prefix = "mak_prov"

	_, err := New(
		WithConfig(newMetricsTestConfig(prefix, true)),
		WithPrometheusRegistrer(reg),
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	mfs := gatherMetricsMap(t, reg)

	expectedMetrics := []string{
		// Middleware-level metrics
		prefix + "_hits",
		prefix + "_requests",
		prefix + "_stale_count",
		prefix + "_collapse_count",
		prefix + "_error_count",
		prefix + "_ttfb_seconds",
		prefix + "_transfer_seconds",

		// Entry storage (Otter) metrics
		prefix + "_entry_otter_in_memory_hits",
		prefix + "_entry_otter_in_memory_gets",
		prefix + "_entry_otter_in_memory_sets",
		prefix + "_entry_otter_in_memory_get_seconds",
		prefix + "_entry_otter_in_memory_set_seconds",

		// Metadata storage (Otter) metrics
		prefix + "_metadata_otter_in_memory_hits",
		prefix + "_metadata_otter_in_memory_gets",
		prefix + "_metadata_otter_in_memory_sets",
		prefix + "_metadata_otter_in_memory_get_seconds",
		prefix + "_metadata_otter_in_memory_set_seconds",
	}

	for _, name := range expectedMetrics {
		if _, ok := mfs[name]; !ok {
			t.Errorf("expected metric %q to be registered", name)
		}
	}
}

func TestMiddleware_Metrics_DuplicatePrefixReturnsError(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := newMetricsTestConfig("mak_dup", false)

	if _, err := New(WithConfig(cfg), WithPrometheusRegistrer(reg)); err != nil {
		t.Fatalf("first New() unexpected error: %v", err)
	}

	if _, err := New(WithConfig(cfg), WithPrometheusRegistrer(reg)); err == nil {
		t.Fatal("second New() with same prefix on same registry expected error, got nil")
	}
}

func TestMiddleware_Metrics_StorageMetricsRecordedOnTraffic(t *testing.T) {
	reg := prometheus.NewRegistry()
	const prefix = "mak_storage"

	mw, err := New(
		WithConfig(newMetricsTestConfig(prefix, false)),
		WithPrometheusRegistrer(reg),
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	origin := new(mock_origin.MockOrigin)
	handler := mw(origin)

	// First request: miss -> metadata get (miss), entry set, metadata set
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/cacheable", nil)
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rec1.Code)
	}

	mfs := gatherMetricsMap(t, reg)
	if v := counterValue(t, mfs, prefix+"_entry_otter_in_memory_gets"); v != 1 {
		t.Errorf("storage gets after miss = %v, want 1", v)
	}
	if v := counterValue(t, mfs, prefix+"_entry_otter_in_memory_hits"); v != 0 {
		t.Errorf("storage hits after miss = %v, want 0", v)
	}
	if v := counterValue(t, mfs, prefix+"_entry_otter_in_memory_sets"); v != 2 {
		t.Errorf("storage sets after miss = %v, want 2 (entry + metadata)", v)
	}

	// Second request: hit -> metadata get (hit), entry get (hit)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/cacheable", nil)
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second request status = %d, want 200", rec2.Code)
	}

	mfs = gatherMetricsMap(t, reg)
	if v := counterValue(t, mfs, prefix+"_entry_otter_in_memory_gets"); v != 3 {
		t.Errorf("storage gets after hit = %v, want 3", v)
	}
	if v := counterValue(t, mfs, prefix+"_entry_otter_in_memory_hits"); v != 2 {
		t.Errorf("storage hits after hit = %v, want 2", v)
	}
	if c := histogramCount(t, mfs, prefix+"_entry_otter_in_memory_get_seconds"); c != 3 {
		t.Errorf("storage get_seconds sample count = %d, want 3", c)
	}
	if c := histogramCount(t, mfs, prefix+"_entry_otter_in_memory_set_seconds"); c != 2 {
		t.Errorf("storage set_seconds sample count = %d, want 2", c)
	}
}

func TestMiddleware_Metrics_HitAndMiss(t *testing.T) {
	reg := prometheus.NewRegistry()
	const prefix = "mak_hitmiss"

	mw, err := New(
		WithConfig(newMetricsTestConfig(prefix, false)),
		WithPrometheusRegistrer(reg),
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	origin := new(mock_origin.MockOrigin)
	handler := mw(origin)

	// Request 1: Miss
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/cacheable", nil))

	mfs := gatherMetricsMap(t, reg)
	if v := counterValue(t, mfs, prefix+"_requests"); v != 1 {
		t.Errorf("requests after first request = %v, want 1", v)
	}
	if v := counterValue(t, mfs, prefix+"_hits"); v != 0 {
		t.Errorf("hits after first request = %v, want 0", v)
	}

	// Request 2: Hit
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/cacheable", nil))

	mfs = gatherMetricsMap(t, reg)
	if v := counterValue(t, mfs, prefix+"_requests"); v != 2 {
		t.Errorf("requests after second request = %v, want 2", v)
	}
	if v := counterValue(t, mfs, prefix+"_hits"); v != 1 {
		t.Errorf("hits after second request = %v, want 1", v)
	}
}

func TestMiddleware_Metrics_Stale(t *testing.T) {
	reg := prometheus.NewRegistry()
	const prefix = "mak_stale"

	baseTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	currentTime := baseTime
	origNow := now
	now = func() time.Time { return currentTime }
	defer func() { now = origNow }()

	mw, err := New(
		WithConfig(newMetricsTestConfig(prefix, false)),
		WithPrometheusRegistrer(reg),
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	origin := new(mock_origin.MockOrigin)
	handler := mw(origin)

	// Populate cache (TTL=10s, MaxStale=30s)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/cacheable", nil))

	// Advance past TTL (10s) but within MaxStale (30s)
	currentTime = baseTime.Add(15 * time.Second)

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/cacheable", nil))

	mfs := gatherMetricsMap(t, reg)
	if v := counterValue(t, mfs, prefix+"_stale_count"); v != 1 {
		t.Errorf("stale_count = %v, want 1", v)
	}
}

func TestMiddleware_Metrics_Collapse(t *testing.T) {
	reg := prometheus.NewRegistry()
	const prefix = "mak_collapse"

	mw, err := New(
		WithConfig(newMetricsTestConfig(prefix, false)),
		WithPrometheusRegistrer(reg),
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := mw(origin)

	const numRequests = 5
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for range numRequests {
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/collapse?collapse_metrics=1", nil)
			handler.ServeHTTP(rec, req)
		}()
	}

	<-started
	time.Sleep(15 * time.Millisecond)
	close(release)
	wg.Wait()

	mfs := gatherMetricsMap(t, reg)
	if v := counterValue(t, mfs, prefix+"_collapse_count"); v < 1 {
		t.Errorf("collapse_count = %v, want >= 1", v)
	}
}

type failingEntryStorage struct {
	err error
}

func (f failingEntryStorage) Get(_ context.Context, _ string) (*cache.Entry, error) {
	return nil, f.err
}
func (f failingEntryStorage) Set(_ context.Context, _ string, _ *cache.Entry) error {
	return f.err
}
func (f failingEntryStorage) Update(_ context.Context, _ string, _ *cache.Entry) error {
	return f.err
}

type failingResponseWriter struct {
	header http.Header
	err    error
}

func (f *failingResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}
func (f *failingResponseWriter) WriteHeader(int)           {}
func (f *failingResponseWriter) Write([]byte) (int, error) { return 0, f.err }

func TestMiddleware_Metrics_Errors(t *testing.T) {
	t.Run("storage failure is treated as a miss, not an error", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		const prefix = "mak_storage_miss"

		h := &Middleware{
			Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
			Config:            newMetricsTestConfig(prefix, false),
			entryStorage:      failingEntryStorage{err: errors.New("storage failure")},
			fwdSingleflight:   new(singleflight.Group),
			revalSingleFlight: new(singleflight.Group),
			metadSingleflight: new(singleflight.Group),
			entrySingleflight: new(singleflight.Group),
		}
		if err := h.ProvisionMetrics(prefix, reg); err != nil {
			t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
		}

		origin := new(mock_origin.MockOrigin)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/cacheable", nil)
		h.ServeHTTP(rec, req, origin)

		mfs := gatherMetricsMap(t, reg)
		if v := counterValue(t, mfs, prefix+"_requests"); v != 1 {
			t.Errorf("requests = %v, want 1", v)
		}
		if v := counterValue(t, mfs, prefix+"_hits"); v != 0 {
			t.Errorf("hits = %v, want 0", v)
		}
		if v := counterValue(t, mfs, prefix+"_error_count"); v != 0 {
			t.Errorf("error_count = %v, want 0", v)
		}
	})

	t.Run("response write failure increments error_count", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		const prefix = "mak_write_errors"

		mw, err := New(
			WithConfig(newMetricsTestConfig(prefix, false)),
			WithPrometheusRegistrer(reg),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		origin := new(mock_origin.MockOrigin)
		handler := mw(origin)

		// Miss with failing client writer -> forward() returns write error
		w1 := &failingResponseWriter{err: errors.New("client write failed")}
		handler.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/cacheable", nil))

		// Hit with failing client writer -> replyWithEntry() returns write error
		w2 := &failingResponseWriter{err: errors.New("client write failed")}
		handler.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/cacheable", nil))

		mfs := gatherMetricsMap(t, reg)
		if v := counterValue(t, mfs, prefix+"_error_count"); v != 2 {
			t.Errorf("error_count = %v, want 2", v)
		}
	})
}

func TestMiddleware_Metrics_Timings(t *testing.T) {
	reg := prometheus.NewRegistry()
	const prefix = "mak_timings"

	mw, err := New(
		WithConfig(newMetricsTestConfig(prefix, false)),
		WithPrometheusRegistrer(reg),
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	origin := new(mock_origin.MockOrigin)
	handler := mw(origin)

	// Miss + Hit = 2 requests
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/cacheable", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/cacheable", nil))

	mfs := gatherMetricsMap(t, reg)
	if c := histogramCount(t, mfs, prefix+"_ttfb_seconds"); c != 2 {
		t.Errorf("ttfb_seconds sample count = %d, want 2", c)
	}
	if c := histogramCount(t, mfs, prefix+"_transfer_seconds"); c != 2 {
		t.Errorf("transfer_seconds sample count = %d, want 2", c)
	}
}
