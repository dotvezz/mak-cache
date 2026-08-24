// Package config provides configuration options for the cache middleware.
// For contributors considering adding functionality to caddy-cache, consider these two things:
//  1. The Go Proverb, "Make the zero value useful"
//  2. The Principle of Least Surprise
//
// Please strive that the zero value for any configuration type is the most sensible default.
package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// ETagConfig holds the configuration for handling ETags. If enabled, ETags from origin will be respected and missing
// ETags will be generated on-the-fly before storing the cached response.
// The default algorithm for hashing is MD5, however this can be overridden by setting either CRC32 or SHA256 to true.
// CRC32 and SHA256 should be considered mutually exclusive and setting both to true is an unsupported configuration.
type ETagConfig struct {
	// Disable turns off all ETag features, including the utilization of ETag headers from origin.
	// In this configuration, ETag headers from origin are still passed downstream, however, they are not stored or used for conditional
	// request handling.
	Disable bool `json:"disable" yaml:"disable"`

	// CRC32 overrides the MD5 default, enabling the use of CRC32 checksums for ETag generation.
	CRC32 bool `json:"crc32" yaml:"crc32"`

	// SHA256 overrides the MD5 default, enabling the use of SHA256 checksums for ETag generation.
	SHA256 bool `json:"sha256" yaml:"sha256"`
}

type CacheKeyConfig struct {
	// Components specifies an ordered list of cache key components to include in the cache key.
	// The default zero/empty value should be interpreted as the default cache key components:
	// - host
	// - path
	// - query
	Components []string `json:"components" yaml:"components"`

	// StripQueryParams specifies a list of query parameters to strip from the cache key.
	StripQueryParams []string `json:"strip_query_params" yaml:"strip_query_params"`

	// NoQuerySort disables sorting of query parameters in the cache key.
	NoQuerySort bool `json:"no_query_sort" yaml:"no_query_sort"`
}

type CoalesceConfig struct {
	// Disable disables coalescing of cache requests. Generally this is not recommended.
	// Coalescing requests blocks concurrent requests until the first one completes, then multiplexes the response down
	// to all waiting downstream connections. It is an important strategy for preventing cache stampedes and protecting
	// origin backends.
	Disable bool `json:"disable" yaml:"disable"`
}

// StorageConfig holds the configuration for a single instance of a storage provider. Each field is mutually exclusive;
// only one field can be non-nil for a single storage instance.
type StorageConfig struct {
	// Otter sets a storage engine/provider to run in memory using Otter
	Otter *OtterConfig `json:"otter,omitempty" yaml:"otter,omitempty"`
	// Valkey sets a storage engine/provider supporting Valkey and Redis (at least Redis Version 6)
	Valkey *ValkeyConfig `json:"valkey,omitempty" yaml:"valkey,omitempty"`
}

type OtterConfig struct {
	MemoryLimit uint64 `json:"memory_limit" yaml:"memory_limit"`
}

type ValkeyConfig struct {
	Address string `json:"address" yaml:"address"`
}

type TimingConfig struct {
	// TTL sets the TTL for cache entries.
	TTL time.Duration `json:"ttl"`

	// MaxStale sets the maximum staleness for cache entries to allow async background refresh.
	MaxStale time.Duration `json:"max_stale"`

	// TTLSplay sets the maximum amount of jitter to add to the TTL sent downstream. This is useful to avoid cache
	// stampedes on synchronized downstream cache expiry.
	// This does not affect the TTL of the cache entry itself. The zero/empty value disables jitter.
	TTLSplay time.Duration `json:"ttl_splay"`
}

func (t TimingConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		TTL      string `json:"ttl,omitempty"`
		MaxStale string `json:"max_stale,omitempty"`
		TTLSplay string `json:"ttl_splay,omitempty"`
	}{
		TTL:      formatDuration(t.TTL),
		MaxStale: formatDuration(t.MaxStale),
		TTLSplay: formatDuration(t.TTLSplay),
	})
}

func (t *TimingConfig) UnmarshalJSON(b []byte) error {
	var aux struct {
		TTL      any `json:"ttl"`
		MaxStale any `json:"max_stale"`
		TTLSplay any `json:"ttl_splay"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	ttl, err := parseDurationValue(aux.TTL)
	if err != nil {
		return fmt.Errorf("invalid ttl: %w", err)
	}
	t.TTL = ttl

	maxStale, err := parseDurationValue(aux.MaxStale)
	if err != nil {
		return fmt.Errorf("invalid max_stale: %w", err)
	}
	t.MaxStale = maxStale

	ttlSplay, err := parseDurationValue(aux.TTLSplay)
	if err != nil {
		return fmt.Errorf("invalid ttl_splay: %w", err)
	}
	t.TTLSplay = ttlSplay

	return nil
}

func (t TimingConfig) MarshalYAML() (any, error) {
	return struct {
		TTL      string `yaml:"ttl,omitempty"`
		MaxStale string `yaml:"max_stale,omitempty"`
		TTLSplay string `yaml:"ttl_splay,omitempty"`
	}{
		TTL:      formatDuration(t.TTL),
		MaxStale: formatDuration(t.MaxStale),
		TTLSplay: formatDuration(t.TTLSplay),
	}, nil
}

func (t *TimingConfig) UnmarshalYAML(value *yaml.Node) error {
	var aux struct {
		TTL      any `yaml:"ttl"`
		MaxStale any `yaml:"max_stale"`
		TTLSplay any `yaml:"ttl_splay"`
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}

	ttl, err := parseDurationValue(aux.TTL)
	if err != nil {
		return fmt.Errorf("invalid ttl: %w", err)
	}
	t.TTL = ttl

	maxStale, err := parseDurationValue(aux.MaxStale)
	if err != nil {
		return fmt.Errorf("invalid max_stale: %w", err)
	}
	t.MaxStale = maxStale

	ttlSplay, err := parseDurationValue(aux.TTLSplay)
	if err != nil {
		return fmt.Errorf("invalid ttl_splay: %w", err)
	}
	t.TTLSplay = ttlSplay

	return nil
}

type RefreshConfig struct {
	// Disable disables stale-while-revalidate or similar features and forces requests to proxy to origin on stale,
	// regardless of origin Cache-Control header directives.
	Disable bool `json:"disable" yaml:"disable"`

	// Timeout specifies the maximum duration to wait for an async background refresh. Zero will default to
	// defaultRefreshTimeout.
	Timeout time.Duration `json:"timeout"`
}

func (r RefreshConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Disable bool   `json:"disable"`
		Timeout string `json:"timeout,omitempty"`
	}{
		Disable: r.Disable,
		Timeout: formatDuration(r.Timeout),
	})
}

func (r *RefreshConfig) UnmarshalJSON(b []byte) error {
	var aux struct {
		Disable bool `json:"disable"`
		Timeout any  `json:"timeout"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	r.Disable = aux.Disable
	to, err := parseDurationValue(aux.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	r.Timeout = to

	return nil
}

func (r RefreshConfig) MarshalYAML() (any, error) {
	return struct {
		Disable bool   `yaml:"disable"`
		Timeout string `yaml:"timeout,omitempty"`
	}{
		Disable: r.Disable,
		Timeout: formatDuration(r.Timeout),
	}, nil
}

func (r *RefreshConfig) UnmarshalYAML(value *yaml.Node) error {
	var aux struct {
		Disable bool `yaml:"disable"`
		Timeout any  `yaml:"timeout"`
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}

	r.Disable = aux.Disable
	to, err := parseDurationValue(aux.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	r.Timeout = to

	return nil
}

type PrometheusConfig struct {
	// Prefix enables Prometheus exporting; unlike most other configurables, metrics are disabled by default.
	// It also defines the prefix to use for metrics names. A prefix is required to enable Prometheus metrics.
	Prefix string `json:"prefix" yaml:"prefix"`
}

type HeadersConfig struct {
	// IgnoreVary specifies a list of headers to ignore when generating the cache key.
	// By default, no Vary headers are ignored.
	IgnoreVary []string `json:"ignore_vary" yaml:"ignore_vary"`

	// OverrideOriginCacheControl sets the handler to override origin Cache-Control header directives with configured
	// values if there are any conflicts, applying configured cache and timing settings even if the origin requests
	// otherwise.
	//
	// While this is useful, it may be undesirable in some cases where the origin cache control headers are important.
	// If the origin service is a third-party vendor, enforcing cache control headers may be against your license
	// terms, for example.
	//
	// Enabling this breaks compliance with RFC 5861 and RFC 9111
	OverrideOriginCacheControl bool `json:"override_origin_cache_control" yaml:"override_origin_cache_control"`

	// OverrideClientCacheControl sets the handler to override request Cache-Control header directives with configured
	// values if there are any conflicts, applying configured cache and timing settings even if the client requests
	// otherwise.
	//
	// Compared to OverrideOriginCacheControl, this is rarely problematic but still not considered a "sane default."
	//
	// Enabling this breaks compliance with RFC 5861 and RFC 9111
	OverrideClientCacheControl bool `json:"override_client_cache_control" yaml:"override_client_cache_control"`
}

type Config struct {
	Timing     TimingConfig     `json:"timing" yaml:"timing"`
	ETag       ETagConfig       `json:"etag" yaml:"etag"`
	Key        CacheKeyConfig   `json:"key" yaml:"key"`
	Coalesce   CoalesceConfig   `json:"coalesce" yaml:"coalesce"`
	Refresh    RefreshConfig    `json:"refresh" yaml:"refresh"`
	Prometheus PrometheusConfig `json:"prometheus" yaml:"prometheus"`
	Headers    HeadersConfig    `json:"headers" yaml:"headers"`

	// StatusTimings allows negative caching for the given HTTP status codes, even if they are not typically cacheable
	StatusTimings map[int]TimingConfig `json:"status_timings,omitempty" yaml:"status_timings,omitempty"`

	// Storage allows for configuring the cache entry storage. At least one Storage must be configured.
	// Multiple Storage configurations can be tied together for a layered setup, for example, a low-latency in-memory
	// layer backed by a redis layer for serving lower-frequency cached values, or as a lazy mechanism for consistency
	// across multiple proxy nodes.
	Storage []StorageConfig `json:"storage" yaml:"storage"`

	// MetadataStorage can optionally be set independently of Storage. If MetadataStorage is defined, it will be used
	// for using metadata lookups. This is useful, for example, to store a large set of cache key metadata (age,
	// vary headers, ETag values, etc) in memory even if the cache entries themselves are kept on disk, in Redis, or in some other
	// storage.
	MetadataStorage []StorageConfig `json:"metadata_storage" yaml:"metadata_storage"`
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

func parseDurationValue(v any) (time.Duration, error) {
	if v == nil {
		return 0, nil
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return 0, nil
		}
		return time.ParseDuration(val)
	case float64:
		return time.Duration(val), nil
	case int64:
		return time.Duration(val), nil
	case int:
		return time.Duration(val), nil
	default:
		return 0, fmt.Errorf("cannot parse %T (%v) as duration", v, v)
	}
}

// UnmarshalJSON on Config handles integer map keys for StatusTimings e.g. "404": {...}
func (c *Config) UnmarshalJSON(b []byte) error {
	type PlainConfig Config
	var aux struct {
		PlainConfig
		StatusTimings map[string]TimingConfig `json:"status_timings"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	*c = Config(aux.PlainConfig)
	if len(aux.StatusTimings) > 0 {
		c.StatusTimings = make(map[int]TimingConfig)
		for k, v := range aux.StatusTimings {
			code, err := strconv.Atoi(k)
			if err != nil {
				return fmt.Errorf("invalid status code key %q: %w", k, err)
			}
			c.StatusTimings[code] = v
		}
	}
	return nil
}
