# mak-cache

[![Go Reference](https://pkg.go.dev/badge/github.com/dotvezz/mak-cache.svg)](https://pkg.go.dev/github.com/dotvezz/mak-cache)

A high-performance, RFC-compliant HTTP caching library for Go, featuring native integration as a [Caddy v2](https://caddyserver.com/) module. `mak-cache` attempts to follow one foundational principle: **The Principle of Least Surprise**: Sane, RFC-compliant defaults out of the box.

It is designed from the ground up for high-throughput, low-latency workloads. It provides cache stampede mitigation, has a storage architecture designed for flexibility and tiered storage layers, decoupled metadata indexing, and strict adherence to modern HTTP caching specifications. 

While keeping overhead minimal through custom binary serialization and zero-allocation memory accounting, its performance under load compares very favorably to Souin and benchmarks close to enterprise caching proxies such as NGINX and Varnish.

---

## Contributing

Any person who wishes to contribute, test and report issues, or otherwise collaborate will be welcomed with open arms! Please do not hesitate to reach out.

### On AI-Assisted Contributions

Disclosure of AI usage in contributions is suggested but not required.

While the core maintainer predominantly maintains this project by hand, it is not an "artisanal" project; it is meant to be used and improved in the real world. AI-assisted contributions will never be rejected solely on the basis of being AI-assisted.

---

## Features

- **Standards Compliance by Default**:
  - **[RFC 9111](https://www.rfc-editor.org/rfc/rfc9111.html)** (HTTP Caching): Expiration model, `Age` calculation, `Cache-Control` response directives (`max-age`, `s-maxage`, `no-cache`, `no-store`, `private`, `public`, `must-revalidate`, `proxy-revalidate`), `Expires` header parsing with `max-age` precedence, and client request directives (`no-cache`, `no-store`, `max-age`, `min-fresh`, `max-stale`).
  - **[RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html)** (HTTP Semantics): Safe method handling (`GET`, `HEAD`), RFC 9110 §15.1 heuristic cacheability status codes (`200`, `203`, `204`, `206`, `300`, `301`, `308`, `404`, `405`, `410`, `414`, `501`), and authorization constraints (authenticated responses are private unless explicitly marked `public`, `s-maxage`, or `must-revalidate`).
  - **[RFC 5861](https://www.rfc-editor.org/rfc/rfc5861.html)** (Stale-While-Revalidate & Stale-If-Error): Serve stale content within the revalidation window while refreshing asynchronously in the background.
  - **[RFC 9211](https://www.rfc-editor.org/rfc/rfc9211.html)** (Cache-Status HTTP Header): Standardized response header indicating cache hit/miss status, forward reasons, TTL, key, and collapsing.
  - **Automatic Invalidation (RFC 9211 §4.4)**: Automatically marks entries and their `Vary`-linked variants for revalidation upon successful unsafe HTTP requests (`POST`, `PUT`, `PATCH`, `DELETE`).
- **Configurable Behavior if Strict Compliance is Undesirable**:
  - Client `Cache-Control` can be ignored to prevent abuse from untrusted clients.
  - Origin `Cache-Control` can be ignored to override with desired cache and timing behavior.
- **Flexible Storage Engines**:
  - **In-Memory**: Powered by [Otter](https://github.com/maypok86/otter) with S3-FIFO eviction, dynamic time-to-evict calculation, and heap size calculation with configurable storage limits.
  - **Distributed**: High-performance distributed key-value storage supporting Valkey or Redis (>= v6) with native TTL propagation.
  - **Tiered Storage**: You can optionally combine a fast L1 in-memory cache with a L2 Redis cache. Cache hits in backing layers automatically promote entries into nearer layers.
  - **Decoupled Metadata Storage**: You can optionally store cache metadata (ETags, Vary headers, expiry times, linked keys) independently from response bodies.
  - **Low-Overhead Serialization**: `mak-cache` ships with a custom high-performance binary serialization format for cache storage, minimizing encoding/decoding overhead.
- **Cache Stampede Protection**:
  - **Request Coalesce / Collapsing**: Uses singleflight deduplication on upstream forward requests and revalidations. Concurrent identical requests wait for a single origin roundtrip, broadcasting the response downstream.
  - **TTL Splay / Jitter**: Configurable downstream TTL jitter to prevent client synchronization triggering thundering herds.
- **Advanced Cache Keying & Normalization**:
  - Configurable key components: `scheme`, `host`, `port`, `method`, `path`, `query`, and arbitrary request headers (`header.<Name>`).
  - Request header canonicalization (`Accept`, `Accept-Language`, `Cache-Control`, and `Accept-Encoding` ranked by compression algorithms) to maximize cache hit rates and prevent key fragmentation.
  - Full `Vary` header support with dynamic secondary cache key generation.
- **Validation & Conditional Requests**:
  - Provides `If-None-Match` conditional requests, as well as performing `If-None-Match` conditional revalidation for stored values.
  - Automatic generation of `304 Not Modified` responses for downstream clients.
  - On-the-fly ETag generation when missing from origin responses (MD5 default, CRC32, or SHA256).
- **Negative Caching (`StatusTimings`)**: **WIP**
  - Configurable custom TTL and stale tolerances for specific HTTP status codes (e.g., short-lived caching for `404 Not Found` or `500 Internal Server Error`).
- **Metrics & Observability**: **WIP**
  - Prometheus metrics support with configurable prefix.

---


### Custom Binary Encoding

Instead of relying on generic serialization formats like bson or gob, `mak-cache` uses an optimized, compact binary protocol:
- **Entries** (`entr\x01`): Serializes headers, status code, metadata, and raw body.
- **Metadata** (`meta\x01`): Serializes ETag, Vary headers, Cache-Control directives, timestamps (`Expires`, `Date`), revalidation flags, and linked Vary keys.

### Tiered Storage & Read-Through Promotion

Storage providers implement the `storage.Provider[T]` interface. When multiple providers are chained using `storage.Wrap`:
- `Get`: Checks the nearest layer (e.g., L1 Otter). On a miss, it checks the backing layer (e.g., L2 Valkey). When an item is found in the backing layer, it is automatically written to the nearer layer.
- `Set` / `Update`: Writes propagate across all configured layers.
  - It's understood that `Set`/`Update` operations will typically be implemented identically, however they are left as separate methods to allow for implementation flexibility and optimization.

---

## Installation

```shell
go get github.com/dotvezz/mak-cache
```

---

## Using as Go HTTP Middleware

`mak-cache` can be integrated into any standard Go HTTP service, router, or framework (such as `net/http`, Chi, Gorilla Mux, Gin, Echo, etc.).

### Quickstart Example

```go
package main

import (
	"fmt"
	"net/http"
	"time"

	mak "github.com/dotvezz/mak-cache"
	"github.com/dotvezz/mak-cache/config"
)

func main() {
	// 1. Configure the cache
	cfg := config.Config{
		Timing: config.TimingConfig{
			TTL:      10 * time.Minute,
			MaxStale: 2 * time.Minute,
		},
		Storage: []config.StorageConfig{
			{
				Otter: &config.OtterConfig{
					MemoryLimit: 128 * 1024 * 1024, // 128 MB in-memory cache
				},
			},
		},
	}

	// 2. Initialize the middleware
	cacheMiddleware, err := mak.New(mak.WithConfig(cfg))
	if err != nil {
		panic(err)
	}

	// 3. Define your application handler
	mux := http.NewServeMux()
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"time":"%s"}`, time.Now().Format(time.RFC3339Nano))
	})

	// 4. Wrap with mak-cache middleware
	handler := cacheMiddleware(mux)

	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", handler)
}
```

### Comprehensive Multi-Tier & Advanced Configuration

```go
package main

import (
	"net/http"
	"time"

	mak "github.com/dotvezz/mak-cache"
	"github.com/dotvezz/mak-cache/config"
)

func setupCache() (func(http.Handler) http.Handler, error) {
	return mak.New(
		mak.WithConfig(config.Config{
			// Default timing rules
			Timing: config.TimingConfig{
				TTL:      15 * time.Minute,
				MaxStale: 5 * time.Minute,      // Stale-While-Revalidate window
				TTLSplay: 10 * time.Second,     // Jitter added to downstream TTL
			},

			// Negative caching for error statuses
			StatusTimings: map[int]config.TimingConfig{
				http.StatusNotFound: {
					TTL: 30 * time.Second,
				},
				http.StatusInternalServerError: {
					TTL: 5 * time.Second,
				},
			},

			// Tiered cache storage: L1 Memory -> L2 Valkey/Redis
			Storage: []config.StorageConfig{
				{
					Otter: &config.OtterConfig{
						MemoryLimit: 256 * 1024 * 1024, // 256MB L1
					},
				},
				{
					Valkey: &config.ValkeyConfig{
						Address: "localhost:6379",      // L2 Shared Valkey/Redis
					},
				},
			},

			// Dedicated in-memory metadata storage for fast key & Vary lookups
			MetadataStorage: []config.StorageConfig{
				{
					Otter: &config.OtterConfig{
						MemoryLimit: 64 * 1024 * 1024, // 64MB Metadata index
					},
				},
			},

			// Cache key configuration
			Key: config.CacheKeyConfig{
				Components:       []string{"host", "path", "query"},
				StripQueryParams: []string{"utm_source", "utm_medium", "utm_campaign", "fbclid"},
				NoQuerySort:      false, // Normalized query parameter ordering
			},

			// Stampede protection
			Coalesce: config.CoalesceConfig{
				Disable: false, // Collapse concurrent misses into a single origin request
			},

			// Asynchronous background refresh
			Refresh: config.RefreshConfig{
				Disable: false,
				Timeout: 10 * time.Second,
			},

			// Automatic on-the-fly ETag generation for responses lacking one
			ETag: config.ETagConfig{
				Disable: false,
				CRC32:   false,
				SHA256:  false, // MD5 by default, or set SHA256/CRC32 to true
			},

			// Header behavior
			Headers: config.HeadersConfig{
				IgnoreVary:                 []string{"User-Agent"},
				OverrideOriginCacheControl: false, // Strictly respect origin by default
				OverrideClientCacheControl: false, // Strictly respect client directives by default
			},

			// Prometheus metrics
			Prometheus: config.PrometheusConfig{
				Prefix: "my_service_cache",
			},
		}),
	)
}
```

---

## RFC 9211 `Cache-Status` Header

Every response processed by `mak-cache` includes an RFC 9211 `Cache-Status` header providing full transparency into cache decisions:

```http
Cache-Status: github.com/dotvezz/mak-cache; hit; ttl=540; key=1da3b6a6967c85e0e85ccb735a3bb77d
```

### Directives:
- `hit`: The response was served directly from cache.
- `ttl=<seconds>`: Remaining lifetime of the cached entry.
- `key=<hash>`: Cache key used for the lookup.
- `collapsed`: The request was coalesced with an in-flight request, preventing origin stampede.
- `stored`: The response was newly stored into cache storage.
- `fwd=<reason>`: The request was forwarded to the upstream origin handler. Reasons include:
  - `uri-miss`: No metadata or cache entry existed for the URI.
  - `vary-miss`: Metadata existed, but headers did not match any stored `Vary` variant.
  - `stale`: Cached entry had expired and was outside the refresh window.
  - `request`: Client request headers forced revalidation (`no-cache`, `max-age=0`, `min-fresh`, etc.).
  - `bypass`: Uncacheable response or request (`no-store`, `private`, `Vary: *`).
  - `method`: Unsafe HTTP method (`POST`, `PUT`, `PATCH`, `DELETE`).
- `fwd-status=<status>`: Upstream origin HTTP status code.

---

## Caddy Plugin

`mak-cache` includes a native plugin for Caddy v2, providing Caddyfile directives and global configuration blocks.

For complete Caddy documentation, installation steps, and Caddyfile examples, see the [Caddy Plugin README](plugins/caddy/readme.md).

---

## Configuration Reference

| Struct Field | Type | Default | Description |
|---|---|---|---|
| `Timing.TTL` | `time.Duration` | `0` | Default time-to-live for cache entries when origin specifies no directives. |
| `Timing.MaxStale` | `time.Duration` | `0` | Stale-While-Revalidate window allowing stale responses during background refresh. |
| `Timing.TTLSplay` | `time.Duration` | `0` | Maximum random duration added to downstream TTL to prevent stampedes. |
| `StatusTimings` | `map[int]TimingConfig` | `nil` | Custom timing rules for specific HTTP status codes (e.g. 404, 500). |
| `Storage` | `[]StorageConfig` | `nil` | One or more storage providers (`Otter` or `Valkey`). Multiple providers form a layered cache. |
| `MetadataStorage` | `[]StorageConfig` | `nil` | Independent storage providers for metadata index lookups. |
| `Key.Components` | `[]string` | `["host", "path", "query"]` | Components forming the cache key (`scheme`, `host`, `port`, `method`, `path`, `query`, `header.<Name>`). |
| `Key.StripQueryParams` | `[]string` | `nil` | Query parameters to strip from the cache key. |
| `Key.NoQuerySort` | `bool` | `false` | Disables sorting/normalization of query string parameters. |
| `Coalesce.Disable` | `bool` | `false` | Disables singleflight request collapsing. |
| `Refresh.Disable` | `bool` | `false` | Disables asynchronous background revalidation. |
| `Refresh.Timeout` | `time.Duration` | `30s` | Maximum duration to wait for background refresh. |
| `ETag.Disable` | `bool` | `false` | Disables ETag handling and generation. |
| `ETag.CRC32` | `bool` | `false` | Use CRC32 for on-the-fly ETag calculation instead of MD5. |
| `ETag.SHA256` | `bool` | `false` | Use SHA256 for on-the-fly ETag calculation instead of MD5. |
| `Headers.IgnoreVary` | `[]string` | `nil` | List of headers to omit when processing origin `Vary`. |
| `Headers.OverrideOriginCacheControl` | `bool` | `false` | Overrides origin Cache-Control headers with configured timing (breaks RFC 9111). |
| `Headers.OverrideClientCacheControl` | `bool` | `false` | Overrides client request Cache-Control directives (breaks RFC 9111). |
| `Prometheus.Prefix` | `string` | `""` | Metrics name prefix. When non-empty, Prometheus metrics are exported. |

---

## Roadmap

- [ ] Surrogate Key / Cache Tag support for targeted group purges
- [ ] Cache purge HTTP endpoints
- [ ] Metrics
- [ ] Full-featured negative caching
- [ ] Full-featured conditional request handling beyond `If-None-Match`

## License

MIT
