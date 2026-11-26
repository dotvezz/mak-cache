package caddy

import (
	"context"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/dotvezz/mak-cache/config"
)

func TestParseFromCustomHelper_PrometheusBlock(t *testing.T) {
	input := `mak_cache {
		prometheus {
			prefix block_prefix
		}
	}`

	d := caddyfile.NewTestDispenser(input)
	d.Next()

	var cfg config.Config
	if err := parseFromCustomHelper(d, &cfg); err != nil {
		t.Fatalf("parseFromCustomHelper() unexpected error: %v", err)
	}
	if cfg.Prometheus.Prefix != "block_prefix" {
		t.Errorf("Prometheus.Prefix = %q, want %q", cfg.Prometheus.Prefix, "block_prefix")
	}
}

func TestHandler_Provision_RegistersMetrics(t *testing.T) {
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	const prefix = "caddy_mak_test"
	h := &Handler{
		Config: config.Config{
			Prometheus: config.PrometheusConfig{
				Prefix: prefix,
			},
			Storage: []config.StorageConfig{
				{
					Otter: &config.OtterConfig{
						MemoryLimit: 10_000_000,
					},
				},
			},
		},
	}

	if err := h.Provision(ctx); err != nil {
		t.Fatalf("Provision() unexpected error: %v", err)
	}

	reg := ctx.GetMetricsRegistry()
	if reg == nil {
		t.Fatal("expected Caddy context to have a non-nil metrics registry")
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() unexpected error: %v", err)
	}

	found := make(map[string]bool, len(mfs))
	for _, mf := range mfs {
		found[mf.GetName()] = true
	}

	expected := []string{
		prefix + "_hits",
		prefix + "_requests",
		prefix + "_stale_count",
		prefix + "_collapse_count",
		prefix + "_error_count",
		prefix + "_ttfb_seconds",
		prefix + "_transfer_seconds",
		prefix + "_entry_otter_in_memory_hits",
		prefix + "_entry_otter_in_memory_gets",
		prefix + "_entry_otter_in_memory_sets",
		prefix + "_entry_otter_in_memory_get_seconds",
		prefix + "_entry_otter_in_memory_set_seconds",
	}

	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected metric %q to be registered in Caddy registry", name)
		}
	}
}
