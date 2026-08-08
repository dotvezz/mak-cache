package cache

import (
	"reflect"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/minitime"
)

func TestParseFromCustomHelper(t *testing.T) {
	input := `cache {
		ignore_vary_headers header1 header2
		timing {
			ttl 10s
			max_stale 5m
			ttl_splay 1s
		}
		status_timings 404 500 {
			ttl 1m
			max_stale 10s
		}
		etag {
			disable false
			crc32 true
			sha256 false
		}
		key {
			components method path query
			strip_query_params query1 query2
			no_query_sort true
		}
		coalesce {
			disable false
		}
		storage otter {
			memory_limit 5000000
		}
		metadata_storage otter {
			memory_limit 10000000
		}
		refresh {
			disable true
			timeout 500ms
		}
		prometheus my_prefix
	}`

	d := caddyfile.NewTestDispenser(input)
	// Consume the outer block/directive name "cache"
	d.Next()

	var cfg config.Config
	err := parseFromCustomHelper(d, &cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify IgnoreVaryHeaders
	expectedVary := []string{"header1", "header2"}
	if !reflect.DeepEqual(cfg.IgnoreVaryHeaders, expectedVary) {
		t.Errorf("expected IgnoreVaryHeaders %v, got %v", expectedVary, cfg.IgnoreVaryHeaders)
	}

	// Verify Timing
	expectedTiming := config.TimingConfig{
		TTL:      minitime.Duration(10 * time.Second),
		MaxStale: minitime.Duration(5 * time.Minute),
		TTLSplay: minitime.Duration(1 * time.Second),
	}
	if !reflect.DeepEqual(cfg.Timing, expectedTiming) {
		t.Errorf("expected Timing %+v, got %+v", expectedTiming, cfg.Timing)
	}

	// Verify StatusTimings
	if len(cfg.StatusTimings) != 2 {
		t.Errorf("expected 2 status timings, got %d", len(cfg.StatusTimings))
	}
	expectedStatusTiming := config.TimingConfig{
		TTL:      minitime.Duration(1 * time.Minute),
		MaxStale: minitime.Duration(10 * time.Second),
	}
	for _, code := range []int{404, 500} {
		if got, ok := cfg.StatusTimings[code]; !ok {
			t.Errorf("missing status timing for %d", code)
		} else if !reflect.DeepEqual(got, expectedStatusTiming) {
			t.Errorf("for code %d: expected status timing %+v, got %+v", code, expectedStatusTiming, got)
		}
	}

	// Verify ETag
	expectedETag := config.ETagConfig{
		Disable: false,
		CRC32:   true,
		SHA256:  false,
	}
	if !reflect.DeepEqual(cfg.ETag, expectedETag) {
		t.Errorf("expected ETag %+v, got %+v", expectedETag, cfg.ETag)
	}

	// Verify Key
	expectedKey := config.CacheKeyConfig{
		Components:       []string{"method", "path", "query"},
		StripQueryParams: []string{"query1", "query2"},
		NoQuerySort:      true,
	}
	if !reflect.DeepEqual(cfg.Key, expectedKey) {
		t.Errorf("expected Key %+v, got %+v", expectedKey, cfg.Key)
	}

	// Verify Coalesce
	expectedCoalesce := config.CoalesceConfig{
		Disable: false,
	}
	if !reflect.DeepEqual(cfg.Coalesce, expectedCoalesce) {
		t.Errorf("expected Coalesce %+v, got %+v", expectedCoalesce, cfg.Coalesce)
	}

	// Verify Storage
	if len(cfg.Storage) != 1 {
		t.Fatalf("expected 1 storage config, got %v", len(cfg.Storage))
	}
	s := cfg.Storage[0]
	if s.Otter == nil {
		t.Fatal("expected Storage.Otter to not be nil")
	}

	// Verify MetadataStorage
	if len(cfg.MetadataStorage) != 1 {
		t.Fatalf("expected 1 metadata storage config, got %v", len(cfg.MetadataStorage))
	}
	m := cfg.MetadataStorage[0]
	if m.Otter == nil {
		t.Fatal("expected MetadataStorage.Otter to not be nil")
	}

	// Verify Refresh
	expectedRefresh := config.RefreshConfig{
		Disable: true,
		Timeout: minitime.Duration(500 * time.Millisecond),
	}
	if !reflect.DeepEqual(cfg.Refresh, expectedRefresh) {
		t.Errorf("expected Refresh %+v, got %+v", expectedRefresh, cfg.Refresh)
	}

	// Verify Prometheus
	expectedPrometheus := config.PrometheusConfig{
		Prefix: "my_prefix",
	}
	if !reflect.DeepEqual(cfg.Prometheus, expectedPrometheus) {
		t.Errorf("expected Prometheus %+v, got %+v", expectedPrometheus, cfg.Prometheus)
	}
}

func TestParseFromCustomHelperErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "unknown option",
			input: `cache {
				unknown_option value
			}`,
		},
		{
			name: "invalid status code",
			input: `cache {
				status_timings abc {
					ttl 10s
				}
			}`,
		},
		{
			name: "missing status code",
			input: `cache {
				status_timings {
					ttl 10s
				}
			}`,
		},
		{
			name: "invalid boolean",
			input: `cache {
				etag {
					disable invalid_bool
				}
			}`,
		},
		{
			name: "invalid duration",
			input: `cache {
				timing {
					ttl 5_invalid
				}
			}`,
		},
		{
			name: "invalid prometheus arguments",
			input: `cache {
				prometheus prefix1 prefix2
			}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := caddyfile.NewTestDispenser(tc.input)
			d.Next()
			var cfg config.Config
			err := parseFromCustomHelper(d, &cfg)
			if err == nil {
				t.Error("expected error but got nil")
			}
		})
	}
}
