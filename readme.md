# caddy-cache

A high performance caching module for [Caddy v2](https://caddyserver.com/), made with care but without a clever name.

This project seeks to be a high-performance, flexible, and feature-filled caching proxy. While a 1:1 comparison is not
truly reasonable, the observed performance under load compares **_very favorably_** to Souin and the Souin-based
caddyserver/cache-handler project. 

## Features

- RFC 9111 Compliance
  - Estimated about 60% complete 
- RFC 9211 Compliance
  - Estimated about 95% complete
- Flexible cache storage options
  - In-memory, with configurable size limits
  - Redis
- Cache Stampede protection features
  - Request Coalescing/Collapsing to mitigate stampedes forwarded to origin
  - Optional TTL Splay/Jitter to mitigate stampedes to the proxy
- ETag Headers and Conditional Requests
- Tiered/Layered cache storage

### Roadmap

- Finish implementation of RFC 9111 
- Finish/polish implementation of RFC 9211
- Implement Negative Caching to protect origins from cache-poisoning-induced thundering herds
- Implement Support for GraphQL
- Implement metrics
- Implement Invalidation and Surrogate Keys

## Contributing

People wishing to contribute, test and report issues, or otherwise collaborate will be welcomed with open arms! Please
do not hesitate to reach out.

## Installation

As with other Caddy v2 plugins, the way to install this module is by building Caddy with it using the [xcaddy tool](https://caddyserver.com/docs/build#xcaddy).

### Build a Single Binary

```shell

go run github.com/caddyserver/xcaddy/cmd/xcaddy \
  --with github.com/dotvezz/caddy-cache

```

### Dockerfile

```Dockerfile
FROM docker.io/caddy:builder-alpine AS build

RUN xcaddy build \
    --with github.com/dotvezz/caddy-cache

FROM docker.io/caddy:alpine as run

COPY --from=build /usr/bin/caddy /usr/bin/caddy
```

## Caddyfile Examples

### Simple In-Memory Cache

The following Caddyfile caches all (cacheable) requests for `api.example.com`, backed by `http://internal-api-host`

```Caddyfile
api.example.com {
  cache {
    timing {
      ttl 1m
    }
    
    storage in-memory {
      memory_limit 134217728 # 128MB
    }
  }
  
  reverse_proxy http://internal-api-host
}
```

### One Shared Cache for Multiple Sites/Routes

It's possible to configure one cache in the server block, which is shared by multiple Caddyfile directive blocks.
The example below shares one 128MB cache across `api.example.com` and `assets.example.com`.

```Caddyfile
{
  cache {
    timing {
      ttl 1m
    }

    storage in-memory {
      memory_limit 134217728 # 128MB
    }
  }
}

api.example.com {
  cache
  reverse_proxy http://internal-api-host
}

assets.example.com {
  cache
  reverse_proxy http://internal-asset-host
}
```

### Multiple Caches across Multiple Sites/Routes

The above example may not perform well in practice; If the asset hostname `assets.example.com` has a large number of
large, high frequency resources, it will lead to high eviction and low cache hit rates for the api hostname
`api.example.com`.

This can be addressed by separating their cache storage so one can't evict the other's responses, But it may still be
useful to share one cache across multiple similar hostnames. 

The following example defines two caches, one called `assets` and one called `api`. The `assets` cache has its default
cache TTL set to 750 hours, and is shared by two asset hostnames `assets1.example.com` and `assets2.example.com`. The
`api` cache only has a 5 minute TTL and is not shared with the asset hostname.

```Caddyfile
{
  cache assets {
    timing {
      ttl 750h # Assuming an asset cache is for static web assets, the cache can live for a long time by default
    }

    storage in-memory {
      memory_limit 134217728 # 128MB
    }
  }

  cache api {
    timing {
      ttl 5m
    }

    storage in-memory {
      memory_limit 134217728 # 128MB
    }
  }
}

api.example.com {
  cache api
  reverse_proxy http://internal-api-host
}

assets1.example.com {
  cache assets
  reverse_proxy http://internal-asset-host
}

assets2.example.com {
  cache assets
  reverse_proxy http://internal-asset-host
}
```


