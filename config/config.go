// Package config provides configuration options for the cache middleware.
// For contributors considering adding functionality to caddy-cache, consider these two things:
//  1. The Go Proverb, "Make the zero value useful"
//  2. The Principle of Least Surprise
//
// Please strive that the zero value for any configuration type is the most sensible default.
package config

import (
	"github.com/dotvezz/caddy-cache/minitime"
)

// ETagConfig holds the configuration for handling ETags. If enabled, ETags from upstream will be respected and missing
// ETags will be generated on-the-fly before storing the cached response.
// The default algorithm for hashing is MD5, however this can be overridden by setting either CRC32 or SHA256 to true.
// CRC32 and SHA256 should be considered mutually exclusive and setting both to true is an unsupported configuration.
type ETagConfig struct {
	// Disable turns off all ETag features, including the utilization of ETag headers from upstream.
	// In this configuration, ETag headers from upstream are still passed downstream, however, they are not stored or used fon conditional
	// request handling.
	Disable bool `json:"disable"`

	// CRC32 overrides the MD5 default, enabling the use of CRC32 checksums for ETag generation.
	CRC32 bool `json:"crc32"`

	// SHA256 overrides the MD5 default, enabling the use of SHA256 checksums for ETag generation.
	SHA256 bool `json:"sha256"`
}

type CacheKeyConfig struct {
	// Components specifies an ordered list of cache key components to include in the cache key.
	// The default zero/empty value should be interpreted as the default cache key components:
	// - method
	// - path
	// - query
	Components []string `json:"components"`

	// StripQueryParams specifies a list of query parameters to strip from the cache key.
	StripQueryParams []string `json:"strip_query_params"`

	// NoQuerySort disables sorting of query parameters in the cache key.
	NoQuerySort bool `json:"no_query_sort"`
}

type CoalesceConfig struct {
	// Disable disables coalescing of cache requests. Generally this is not recommended.
	// Coalescing requests blocks concurrent requests until the first one completes, then multiplexes the response down
	// to all waiting downstream connections. It is an important strategy for preventing cache stampedes and protecting
	// upstream backends.
	Disable bool `json:"disable"`
}

// StorageConfig holds the configuration for a single instance of a storage provider. Each field is mutually exclusive;
// only one field can be non-nil for a single storage instance.
type StorageConfig struct {
	// Otter sets a storage engine/provider to run in memory using Otter
	Otter *OtterConfig `json:"otter,omitempty"`
}

type OtterConfig struct {
	MemoryLimit uint64 `json:"memory_limit"`
}

type TimingConfig struct {
	// TTL sets the TTL for cache entries.
	TTL minitime.Duration `json:"ttl"`

	// MaxStale sets the maximum staleness for cache entries to allow async background refresh.
	MaxStale minitime.Duration `json:"max_stale"`

	// TTLSplay sets the maximum amount of jitter to add to the TTL sent downstream. This is useful to avoid cache
	// stampedes on synchronized downstream cache expiry.
	// This does not affect the TTL of the cache entry itself. The zero/empty value disables jitter.
	TTLSplay minitime.Duration `json:"ttl_splay"`
}

type RefreshConfig struct {
	// Disable disables refresh-while-stale features and forces requests to proxy upstream on stale instead.
	Disable bool `json:"disable"`

	// Timeout specifies the maximum duration to wait for an async background refresh. Zero will default to
	// defaultRefreshTimeout
	Timeout minitime.Duration `json:"timeout"`
}

type PrometheusConfig struct {
	// Prefix enables Prometheus exporting; unlike most other configurables, metrics are disabled by default.
	// It also defines the prefix to use for metrics names. A prefix is required to enable Prometheus metrics.
	Prefix string `json:"prefix"`
}

type Config struct {
	Timing     TimingConfig     `json:"timing"`
	ETag       ETagConfig       `json:"etag"`
	Key        CacheKeyConfig   `json:"key"`
	Coalesce   CoalesceConfig   `json:"coalesce"`
	Refresh    RefreshConfig    `json:"refresh"`
	Prometheus PrometheusConfig `json:"prometheus"`

	// StatusTimings allows negative caching for the given HTTP status codes, even if they are not typically cacheable
	StatusTimings map[int]TimingConfig `json:"status_timings,omitempty"`

	// Storage allows for configuring the cache value storage. At least one Storage must be configured.
	// Multiple Storage configurations can be tied together for a layered setup, for example a low-latency in-memory
	// layer backed by a redis layer for serving lower-frequency cached values, or as a lazy mechanism for consistency
	// across multiple proxy nodes.
	Storage []StorageConfig `json:"storage"`

	// MetadataStorage can optionally be set independently of Storage. If MetadataStorage is defined, it will be used
	// for using metadata lookups. This is useful, for example, to store a large set of cache key metadata (age,
	// vary headers, ETag values, etc) in memory even if the cache entries themselves are kept on disk, in Redis, or in some other
	// storage.
	MetadataStorage []StorageConfig `json:"metadata_storage"`

	// IgnoreVaryHeaders specifies a list of headers to ignore when generating the cache key.
	// By default, no Vary headers are ignored.
	IgnoreVaryHeaders []string `json:"ignore_vary_headers"`
}
