package valkey

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/metrics"
	"github.com/dotvezz/mak-cache/storage"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	vkey "github.com/valkey-io/valkey-go"
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
	_ metrics.Provisioner = (*Provider[cache.Entry, *cache.Entry])(nil)
	_ metrics.Provisioner = (*Provider[cache.Metadata, *cache.Metadata])(nil)
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

type failingMarshalStorable struct {
	marshalErr error
}

func (f *failingMarshalStorable) MarshalTo(_ cache.Writer) error {
	return f.marshalErr
}

func (f *failingMarshalStorable) Unmarshal(_ cache.Reader) error {
	return nil
}

func (f *failingMarshalStorable) EvictAt() time.Time {
	return time.Now().Add(time.Hour)
}

func (f *failingMarshalStorable) HeapSize() int    { return 0 }
func (f *failingMarshalStorable) RefreshHeapSize() {}

func TestProvider_ProvisionMetrics_Entry(t *testing.T) {
	p := &Provider[cache.Entry, *cache.Entry]{client: &mockClient{}}
	reg := prometheus.NewRegistry()

	if err := p.ProvisionMetrics("mak", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	got := gatherMap(t, reg)
	want := map[string]dto.MetricType{
		"mak_entry_redis_valkey_hits":        dto.MetricType_COUNTER,
		"mak_entry_redis_valkey_gets":        dto.MetricType_COUNTER,
		"mak_entry_redis_valkey_sets":        dto.MetricType_COUNTER,
		"mak_entry_redis_valkey_get_errors":  dto.MetricType_COUNTER,
		"mak_entry_redis_valkey_set_errors":  dto.MetricType_COUNTER,
		"mak_entry_redis_valkey_get_seconds": dto.MetricType_HISTOGRAM,
		"mak_entry_redis_valkey_set_seconds": dto.MetricType_HISTOGRAM,
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
	p := &Provider[cache.Metadata, *cache.Metadata]{client: &mockClient{}}
	reg := prometheus.NewRegistry()

	if err := p.ProvisionMetrics("mak", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	got := gatherMap(t, reg)
	want := map[string]dto.MetricType{
		"mak_metadata_redis_valkey_hits":        dto.MetricType_COUNTER,
		"mak_metadata_redis_valkey_gets":        dto.MetricType_COUNTER,
		"mak_metadata_redis_valkey_sets":        dto.MetricType_COUNTER,
		"mak_metadata_redis_valkey_get_errors":  dto.MetricType_COUNTER,
		"mak_metadata_redis_valkey_set_errors":  dto.MetricType_COUNTER,
		"mak_metadata_redis_valkey_get_seconds": dto.MetricType_HISTOGRAM,
		"mak_metadata_redis_valkey_set_seconds": dto.MetricType_HISTOGRAM,
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
	p1 := &Provider[cache.Entry, *cache.Entry]{client: &mockClient{}}
	p2 := &Provider[cache.Entry, *cache.Entry]{client: &mockClient{}}
	reg := prometheus.NewRegistry()

	if err := p1.ProvisionMetrics("dup", reg); err != nil {
		t.Fatalf("first ProvisionMetrics() unexpected error: %v", err)
	}
	if err := p2.ProvisionMetrics("dup", reg); err == nil {
		t.Fatal("second ProvisionMetrics() expected error, got nil")
	}
}

func TestProvider_Metrics_GetHitMissAndErrors(t *testing.T) {
	entry := &cache.Entry{
		Status: 200,
		Body:   []byte("valkey body"),
	}
	buf := bytes.NewBuffer(nil)
	_ = entry.MarshalTo(buf)
	entryBytes := buf.Bytes()

	var nextResult vkey.ValkeyResult
	mock := &mockClient{
		doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
			return nextResult
		},
	}

	p := &Provider[cache.Entry, *cache.Entry]{client: mock, timeout: time.Second}
	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("vk", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	ctx := context.Background()

	// 1. Hit
	nextResult = newMockResult(entryBytes, nil)
	if _, err := p.Get(ctx, "k1"); err != nil {
		t.Fatalf("Get(hit) unexpected error: %v", err)
	}
	if v := counterVal(t, p.gets); v != 1 {
		t.Errorf("gets after hit = %v, want 1", v)
	}
	if v := counterVal(t, p.hits); v != 1 {
		t.Errorf("hits after hit = %v, want 1", v)
	}
	if v := counterVal(t, p.getErrors); v != 0 {
		t.Errorf("getErrors after hit = %v, want 0", v)
	}

	// 2. Miss (vkey.Nil -> storage.ErrNotFound, should NOT count as error)
	nextResult = newMockResult(nil, vkey.Nil)
	if _, err := p.Get(ctx, "k2"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get(miss) error = %v, want ErrNotFound", err)
	}
	if v := counterVal(t, p.gets); v != 2 {
		t.Errorf("gets after miss = %v, want 2", v)
	}
	if v := counterVal(t, p.hits); v != 1 {
		t.Errorf("hits after miss = %v, want 1", v)
	}
	if v := counterVal(t, p.getErrors); v != 0 {
		t.Errorf("getErrors after miss = %v, want 0", v)
	}

	// 3. Client error
	nextResult = newMockResult(nil, errors.New("connection refused"))
	if _, err := p.Get(ctx, "k3"); err == nil {
		t.Fatal("Get(client err) expected error, got nil")
	}
	if v := counterVal(t, p.gets); v != 3 {
		t.Errorf("gets after client err = %v, want 3", v)
	}
	if v := counterVal(t, p.hits); v != 1 {
		t.Errorf("hits after client err = %v, want 1", v)
	}
	if v := counterVal(t, p.getErrors); v != 1 {
		t.Errorf("getErrors after client err = %v, want 1", v)
	}

	// 4. Unmarshal error
	nextResult = newMockResult([]byte("corrupted_bytes"), nil)
	if _, err := p.Get(ctx, "k4"); err == nil {
		t.Fatal("Get(unmarshal err) expected error, got nil")
	}
	if v := counterVal(t, p.gets); v != 4 {
		t.Errorf("gets after unmarshal err = %v, want 4", v)
	}
	if v := counterVal(t, p.hits); v != 1 {
		t.Errorf("hits after unmarshal err = %v, want 1", v)
	}
	if v := counterVal(t, p.getErrors); v != 2 {
		t.Errorf("getErrors after unmarshal err = %v, want 2", v)
	}

	mfs := gatherMap(t, reg)
	getHist := mfs["vk_entry_redis_valkey_get_seconds"].GetMetric()[0].GetHistogram()
	if getHist.GetSampleCount() != 4 {
		t.Errorf("get_seconds sample count = %d, want 4", getHist.GetSampleCount())
	}
}

func TestProvider_Metrics_SetAndUpdateSuccessAndErrors(t *testing.T) {
	var nextResult vkey.ValkeyResult
	mock := &mockClient{
		doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
			return nextResult
		},
	}

	p := &Provider[cache.Entry, *cache.Entry]{client: mock, timeout: time.Second}
	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("vkset", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	entry := &cache.Entry{
		Status: 200,
		Body:   []byte("payload"),
		Metadata: cache.Metadata{
			Expires: time.Now().Add(time.Hour),
			Evict:   time.Now().Add(2 * time.Hour),
		},
	}
	ctx := context.Background()

	// 1. Successful Set
	nextResult = newMockResult(nil, nil)
	if err := p.Set(ctx, "k1", entry); err != nil {
		t.Fatalf("Set() unexpected error: %v", err)
	}
	if v := counterVal(t, p.sets); v != 1 {
		t.Errorf("sets after Set = %v, want 1", v)
	}
	if v := counterVal(t, p.setErrors); v != 0 {
		t.Errorf("setErrors after Set = %v, want 0", v)
	}

	// 2. Successful Update
	if err := p.Update(ctx, "k1", entry); err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if v := counterVal(t, p.sets); v != 2 {
		t.Errorf("sets after Update = %v, want 2", v)
	}
	if v := counterVal(t, p.setErrors); v != 0 {
		t.Errorf("setErrors after Update = %v, want 0", v)
	}

	// 3. Client error on Set
	nextResult = newMockResult(nil, errors.New("write timeout"))
	if err := p.Set(ctx, "k1", entry); err == nil {
		t.Fatal("Set(client err) expected error, got nil")
	}
	if v := counterVal(t, p.sets); v != 3 {
		t.Errorf("sets after client err = %v, want 3", v)
	}
	if v := counterVal(t, p.setErrors); v != 1 {
		t.Errorf("setErrors after client err = %v, want 1", v)
	}

	mfs := gatherMap(t, reg)
	setHist := mfs["vkset_entry_redis_valkey_set_seconds"].GetMetric()[0].GetHistogram()
	if setHist.GetSampleCount() != 3 {
		t.Errorf("set_seconds sample count = %d, want 3", setHist.GetSampleCount())
	}
}

func TestProvider_Metrics_SetMarshalError(t *testing.T) {
	p := &Provider[failingMarshalStorable, *failingMarshalStorable]{
		client:  &mockClient{},
		timeout: time.Second,
	}
	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("vkmarshal", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	err := p.Set(context.Background(), "k1", &failingMarshalStorable{
		marshalErr: errors.New("marshal boom"),
	})
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if v := counterVal(t, p.sets); v != 1 {
		t.Errorf("sets after marshal err = %v, want 1", v)
	}
	if v := counterVal(t, p.setErrors); v != 1 {
		t.Errorf("setErrors after marshal err = %v, want 1", v)
	}
}

func TestProvider_Metrics_TimingObservation(t *testing.T) {
	mock := &mockClient{
		doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
			return newMockResult(nil, vkey.Nil)
		},
	}
	p := &Provider[cache.Entry, *cache.Entry]{client: mock, timeout: time.Second}
	reg := prometheus.NewRegistry()
	if err := p.ProvisionMetrics("vktiming", reg); err != nil {
		t.Fatalf("ProvisionMetrics() unexpected error: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	origNow := now
	now = func() time.Time {
		calls++
		return base.Add(time.Duration(calls) * 10 * time.Millisecond)
	}
	defer func() { now = origNow }()

	_, _ = p.Get(context.Background(), "miss")

	mfs := gatherMap(t, reg)
	getHist := mfs["vktiming_entry_redis_valkey_get_seconds"].GetMetric()[0].GetHistogram()
	if getHist.GetSampleCount() != 1 {
		t.Fatalf("get_seconds sample count = %d, want 1", getHist.GetSampleCount())
	}
	if getHist.GetSampleSum() < 0.009 || getHist.GetSampleSum() > 0.011 {
		t.Errorf("get_seconds sample sum = %v, want ~0.010s", getHist.GetSampleSum())
	}
}

func TestProvider_Metrics_WithoutProvisioning(t *testing.T) {
	mock := &mockClient{
		doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
			return newMockResult(nil, vkey.Nil)
		},
	}
	p := &Provider[cache.Entry, *cache.Entry]{client: mock, timeout: time.Second}

	_, err := p.Get(context.Background(), "k1")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}

	mock.doFunc = func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
		return newMockResult(nil, nil)
	}
	entry := &cache.Entry{
		Status: 200,
		Metadata: cache.Metadata{
			Expires: time.Now().Add(time.Hour),
			Evict:   time.Now().Add(2 * time.Hour),
		},
	}
	if err := p.Set(context.Background(), "k1", entry); err != nil {
		t.Fatalf("Set() unexpected error: %v", err)
	}
}
