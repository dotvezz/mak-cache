# mak-cache

A high-performance HTTP caching library for Go, including a plugin for Caddy

This project aims to be a high-performance, flexible, and feature-filled caching proxy. While a 1:1 comparison is not
truly reasonable, the observed performance under load compares **_very favorably_** to Souin, and is much closer to
enterprise HTTP caching proxies like nginx and Varnish.

## Features

- Compliant By Default with RFC 5861, 9110, 9111, and 9211
- Flexible cache storage options
  - In-memory, with configurable size limits
  - Redis >= V6
  - Valkey
- Cache Stampede protection features
  - Request Coalescing/Collapsing to mitigate stampedes forwarded to origin
  - Optional TTL Splay/Jitter to mitigate stampedes to the proxy
- Support for ETag Revalidation with `If-None-Match` Conditional requests
- Tiered/Layered cache storage

### Roadmap

- Implement metrics
- Implement support for Surrogate Keys

## Contributing

People wishing to contribute, test and report issues, or otherwise collaborate will be welcomed with open arms! Please
do not hesitate to reach out.

## Using

### As a Go HTTP Middleware

Examples coming

### In Caddy

You can read the Caddy plugin readme.
