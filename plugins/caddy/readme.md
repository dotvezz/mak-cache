# mak-cache/plugins/caddy

A high-performance HTTP caching module for [Caddy v2](https://caddyserver.com/) (`http.handlers.cache`), built for real-world use with precision and care.

`mak-cache` brings enterprise-grade HTTP caching to Caddy, featuring multi-tier storage (in-memory + Valkey/Redis), request coalescing (singleflight stampede protection), decoupled metadata indexing, dynamic `Vary` support, RFC 5861 background stale-while-revalidate refreshes, and full RFC 9111 / RFC 9211 compliance.

Observed performance under load compares very favorably to Souin and the Souin-based `cache-handler` plugin, offering significantly reduced memory overhead and predictable latencies in-line with NGINX and Varnish.

---

## Installation

Like other Caddy modules, you compile Caddy with `mak-cache` using [xcaddy](https://caddyserver.com/docs/build#xcaddy).

### Build a Single Binary

```shell
xcaddy build \
  --with github.com/dotvezz/mak-cache/plugins/caddy
```

Or run directly via Go:

```shell
go run github.com/caddyserver/xcaddy/cmd/xcaddy build \
  --with github.com/dotvezz/mak-cache/plugins/caddy
```

### Dockerfile Example

```dockerfile
FROM docker.io/caddy:builder-alpine AS build

RUN xcaddy build \
    --with github.com/dotvezz/mak-cache/plugins/caddy

FROM docker.io/caddy:alpine AS run

COPY --from=build /usr/bin/caddy /usr/bin/caddy
```

---

## Directive Placement & Ordering

The module registers the `cache` directive automatically in Caddy's HTTP handler chain **before `rewrite`**:

```
cache -> rewrite -> ... -> reverse_proxy
```

This ensures that incoming requests are checked against the cache before rewrites or proxy roundtrips, and responses from upstream handlers (e.g. `reverse_proxy`, `file_server`) are captured, evaluated for cacheability, and served efficiently.

---

## Caddyfile Configuration Reference

The `cache` directive can be configured:
1. **Globally**: Inside the top-level global options block `{ ... }`. This allows sharing a single cache instance across multiple site blocks or defining named cache profiles.
2. **Per Site / Route**: Inside a site block `example.com { ... }` or route handler block.

### Syntax Overview

```Caddyfile
cache [optional_profile_name] {
    timing {
        ttl <duration>
        max_stale <duration>
        ttl_splay <duration>
    }

    status_timings <status_codes...> {
        ttl <duration>
        max_stale <duration>
        ttl_splay <duration>
    }

    storage <in_memory|otter> <size>
    # OR
    storage <valkey|redis> <address>

    metadata <in_memory|otter|valkey|redis> <config>

    key {
        components <component...>
        strip_query_params <param...>
        no_query_sort [bool]
    }

    coalesce {
        disable [bool]
    }

    refresh {
        disable [bool]
        timeout <duration>
    }

    etag {
        disable [bool]
        crc32 [bool]
        sha256 [bool]
    }

    headers {
        ignore_vary <header...>
        override_origin_cache_control [bool]
        override_client_cache_control [bool]
    }

    prometheus [prefix]
}
```

---

### Configuration Directives

#### `timing`
Defines default timing rules applied when the upstream origin does not supply explicit `Cache-Control` directives.
- `ttl <duration>`: Default TTL for cache entries (e.g., `10m`, `1h`, `750h`).
- `max_stale <duration>`: Maximum staleness allowed for serving cached entries while revalidating asynchronously in the background (RFC 5861 `stale-while-revalidate`).
- `ttl_splay <duration>`: Maximum random jitter added to the downstream response `TTL` to prevent synchronized cache expirations across downstream clients.

#### `status_timings` / `status_timing`
Configures negative caching and status-specific TTLs for one or more HTTP status codes:
```Caddyfile
status_timings 404 410 {
    ttl 1m
    max_stale 10s
}

status_timings 500 502 503 {
    ttl 5s
}
```

#### `storage` / `entries` / `entry_storage`
Configures where cache response bodies and entries are stored. At least one storage backend must be configured.
- **In-Memory (Otter)**:
  ```Caddyfile
  storage in_memory 256MB
  # Or with otter alias and block syntax:
  storage otter {
      memory_limit 256MB
  }
  ```
  Sizes accept human-readable byte formats (e.g., `128MB`, `2GB`) or raw integer bytes. Powered by [Otter](https://github.com/maypok86/otter) with S3-FIFO eviction and size-class heap accounting.
- **Distributed (Valkey / Redis)**:
  ```Caddyfile
  storage valkey localhost:6379
  # Or with redis alias and block syntax:
  storage redis {
      address localhost:6379
  }
  ```
- **Tiered / Layered Storage**:
  You can declare multiple storage backends. Defining an in-memory layer followed by a Valkey layer automatically chains them:
  ```Caddyfile
  entries in_memory 128MB
  entries valkey redis-cluster.internal:6379
  ```
  Cache lookups check L1 (in-memory) first. If L1 misses and L2 (Valkey) hits, the entry is automatically promoted back into L1. Writes and updates propagate across all tiers.

#### `metadata` / `metadata_storage`
Configures a dedicated storage provider for cache key metadata (Vary headers, ETags, expiration timestamps, linked variants).
```Caddyfile
metadata in_memory 64MB
entries valkey localhost:6379
```
*Why use dedicated metadata storage?* It allows Caddy to keep metadata for millions of keys in ultra-fast local memory, avoiding remote Valkey/Redis network hops for cache misses, Vary evaluations, and conditional request checks.

#### `key`
Controls how cache keys are computed from incoming requests:
- `components <list...>`: Ordered list of components included in the key. Defaults to `host path query`. Supported components:
  - `scheme`: URL scheme (`http`, `https`).
  - `host`: Hostname (lowercase, port stripped).
  - `port`: Host port.
  - `method`: HTTP verb (`GET`, `HEAD`, etc.).
  - `path`: Request URL path.
  - `query`: Query string parameters.
  - `header.<Header-Name>`: Include a specific request header (e.g., `header.Accept`, `header.X-Country-Code`).
- `strip_query_params <param...>`: Strips specific query string parameters from the cache key (e.g., `utm_source`, `utm_medium`, `utm_campaign`, `fbclid`).
- `no_query_sort [bool]`: Disables alphabetical sorting of query string parameters. Default is `false` (parameters are sorted to normalize keys).

#### `coalesce`
Protects origin backends from cache stampedes:
- `disable [bool]`: When `false` (default), concurrent identical cache-miss requests are collapsed into a single upstream origin request using `singleflight`. Waiting requests receive a multiplexed copy of the origin response. Set to `true` to disable.

#### `refresh`
Controls asynchronous background revalidation (RFC 5861 `stale-while-revalidate`):
- `disable [bool]`: Disables background revalidation, forcing stale requests to block on origin revalidation.
- `timeout <duration>`: Maximum duration to allow for an asynchronous background revalidation request (defaults to `30s`).

#### `etag`
Controls HTTP entity tag processing:
- `disable [bool]`: Disables storing and validating ETags.
- `crc32 [bool]`: Generates missing ETags on-the-fly using CRC32 checksums.
- `sha256 [bool]`: Generates missing ETags on-the-fly using SHA-256.
*(Default is MD5 if neither `crc32` nor `sha256` is enabled).*

#### `headers`
Adjusts header evaluation and HTTP specification compliance:
- `ignore_vary <header...>`: Ignore specific headers declared by the origin's `Vary` header (e.g., `ignore_vary User-Agent`).
- `override_origin_cache_control [bool]`: Force configured cache TTL and timing even if origin returns `no-cache`, `no-store`, or `private`. *(Warning: breaks RFC 9111 compliance).*
- `override_client_cache_control [bool]`: Ignore client-side cache bypass directives such as `no-cache` or `max-age=0`. *(Warning: breaks RFC 9111 compliance).*

#### `prometheus`
Enables Prometheus metric instrumentation:
```Caddyfile
prometheus my_cache
# Or:
prometheus {
    prefix my_cache
}
```

---

## Caddyfile Usage Scopes

### 1. Simple In-Memory Site Cache

Caches responses for `api.example.com` backed by an upstream reverse proxy:

```Caddyfile
api.example.com {
    cache {
        timing {
            ttl 10m
            max_stale 2m
            ttl_splay 5s
        }

        storage in_memory 256MB
    }

    reverse_proxy http://internal-api:8000
}
```

### 2. Global Shared Cache Across Multiple Sites

Define a global cache in the top-level options block. Multiple site blocks reference it, sharing the same in-memory cache instance:

```Caddyfile
{
    cache {
        timing {
            ttl 5m
            max_stale 1m
        }

        storage in_memory 512MB
    }
}

api.example.com {
    cache
    reverse_proxy http://api-service:8080
}

web.example.com {
    cache
    reverse_proxy http://web-service:3000
}
```

### 3. Named Profiles for Different Workloads

Define distinct cache profiles with different TTLs and capacities in the global block, then reference them by name:

```Caddyfile
{
    # Long-lived static assets cache
    cache assets {
        timing {
            ttl 720h # 30 days
        }

        storage in_memory 1GB
    }

    # Short-lived dynamic API cache with singleflight coalescing
    cache api {
        timing {
            ttl 2m
            max_stale 30s
            ttl_splay 3s
        }

        status_timings 404 {
            ttl 30s
        }

        storage in_memory 256MB
    }
}

static1.example.com {
    cache assets
    reverse_proxy http://cdn-origin:8080
}

static2.example.com {
    cache assets
    reverse_proxy http://cdn-origin:8080
}

api.example.com {
    cache api
    reverse_proxy http://api-origin:8080
}
```

### 4. Enterprise Two-Tier Caching (L1 Memory + L2 Valkey/Redis)

Combines fast local RAM with a shared distributed Valkey/Redis instance and in-memory metadata:

```Caddyfile
{
    cache {
        timing {
            ttl 30m
            max_stale 5m
        }

        # Fast local metadata index
        metadata in_memory 64MB

        # Layer 1: In-memory cache (fastest)
        entries in_memory 256MB

        # Layer 2: Distributed Valkey/Redis cluster
        entries valkey valkey.internal.net:6379
    }
}

api.example.com {
    cache
    reverse_proxy http://api-backend:8080
}
```

### 5. Production API Gateway with Query Stripping & Negative Caching

```Caddyfile
api.example.com {
    cache {
        timing {
            ttl 15m
            max_stale 2m
            ttl_splay 5s
        }

        # Cache 404s briefly, but avoid caching 5xx errors
        status_timings 404 {
            ttl 30s
        }

        # Strip marketing / tracking parameters to increase cache hit ratio
        key {
            strip_query_params utm_source utm_medium utm_campaign fbclid gclid
        }

        storage in_memory 512MB

        etag {
            sha256 true
        }
    }

    reverse_proxy http://api-cluster:9000
}
```

---

## Response Verification: RFC 9211 `Cache-Status`

To observe cache performance, inspect the `Cache-Status` header on HTTP responses:

```shell
curl -I https://api.example.com/items/123
```

Example response headers:

```http
HTTP/2 200
Content-Type: application/json
Age: 42
Expires: Thu, 03 Sep 2026 18:50:00 GMT
ETag: "9a2f64c178..."
Cache-Status: github.com/dotvezz/mak-cache; hit; ttl=558; key=98b3ca...
```

On a collapsed cache miss:
```http
Cache-Status: github.com/dotvezz/mak-cache; collapsed; fwd=uri-miss; fwd-status=200; stored; key=98b3ca...
```
