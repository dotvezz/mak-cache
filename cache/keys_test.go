package cache_test

import (
	"net/http"
	"testing"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/config"
)

func TestGenerateKey(t *testing.T) {
	t.Run("nil request returns empty string", func(t *testing.T) {
		key := cache.GenerateKey(nil, config.CacheKeyConfig{}, nil)
		if key != "" {
			t.Errorf("got %q, want empty string", key)
		}
	})

	t.Run("default components (host, path, query)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://example.com:8080/foo/bar?b=2&a=1", nil)
		cfg := config.CacheKeyConfig{}
		key1 := cache.GenerateKey(req, cfg, nil)
		if key1 == "" {
			t.Fatal("expected non-empty key")
		}

		// Same host, path, query (different query parameter order) -> sorted query produces same key
		req2, _ := http.NewRequest("GET", "http://example.com:8080/foo/bar?a=1&b=2", nil)
		key2 := cache.GenerateKey(req2, cfg, nil)
		if key1 != key2 {
			t.Errorf("expected key1 (%s) == key2 (%s)", key1, key2)
		}
	})

	t.Run("custom components: scheme, host, port, method, path, query, header", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "https://example.com:8080/foo/bar?param=1", nil)
		req.Header.Set("X-Custom-Header", "my-value")

		cfg := config.CacheKeyConfig{
			Components: []string{"scheme", "host", "port", "method", "path", "query", "header.x-custom-header"},
		}
		key1 := cache.GenerateKey(req, cfg, nil)
		if key1 == "" {
			t.Fatal("expected non-empty key")
		}

		// Changing method should alter key when 'method' component is included
		reqGet, _ := http.NewRequest("GET", "https://example.com:8080/foo/bar?param=1", nil)
		reqGet.Header.Set("X-Custom-Header", "my-value")
		keyGet := cache.GenerateKey(reqGet, cfg, nil)
		if key1 == keyGet {
			t.Errorf("expected different key when method changes")
		}
	})

	t.Run("NoQuerySort flag", func(t *testing.T) {
		cfgSorted := config.CacheKeyConfig{NoQuerySort: false}
		cfgUnsorted := config.CacheKeyConfig{NoQuerySort: true}

		req1, _ := http.NewRequest("GET", "http://example.com/test?b=2&a=1", nil)
		req2, _ := http.NewRequest("GET", "http://example.com/test?a=1&b=2", nil)

		// With sorting (default), different param order produces same key
		keySorted1 := cache.GenerateKey(req1, cfgSorted, nil)
		keySorted2 := cache.GenerateKey(req2, cfgSorted, nil)
		if keySorted1 != keySorted2 {
			t.Errorf("sorted query string keys should match: %s != %s", keySorted1, keySorted2)
		}

		// With NoQuerySort=true, param order is preserved
		keyUnsorted1 := cache.GenerateKey(req1, cfgUnsorted, nil)
		keyUnsorted2 := cache.GenerateKey(req2, cfgUnsorted, nil)
		if keyUnsorted1 == keyUnsorted2 {
			t.Errorf("unsorted query string keys should differ when order differs")
		}
	})

	t.Run("StripQueryParams", func(t *testing.T) {
		cfg := config.CacheKeyConfig{
			StripQueryParams: []string{"utm_source", "fbclid"},
		}

		req1, _ := http.NewRequest("GET", "http://example.com/page?id=123&utm_source=google", nil)
		req2, _ := http.NewRequest("GET", "http://example.com/page?id=123", nil)

		key1 := cache.GenerateKey(req1, cfg, nil)
		key2 := cache.GenerateKey(req2, cfg, nil)
		if key1 != key2 {
			t.Errorf("stripped parameter should produce matching keys: %s != %s", key1, key2)
		}

		// Test NoQuerySort with StripQueryParams
		cfgUnsorted := config.CacheKeyConfig{
			NoQuerySort:      true,
			StripQueryParams: []string{"utm_source"},
		}
		keyUnsorted1 := cache.GenerateKey(req1, cfgUnsorted, nil)
		keyUnsorted2 := cache.GenerateKey(req2, cfgUnsorted, nil)
		if keyUnsorted1 != keyUnsorted2 {
			t.Errorf("unsorted stripped parameter should produce matching keys: %s != %s", keyUnsorted1, keyUnsorted2)
		}
	})

	t.Run("Vary headers integration", func(t *testing.T) {
		cfg := config.CacheKeyConfig{}

		reqGzip, _ := http.NewRequest("GET", "http://example.com/page", nil)
		reqGzip.Header.Set("Accept-Encoding", "gzip")

		reqBr, _ := http.NewRequest("GET", "http://example.com/page", nil)
		reqBr.Header.Set("Accept-Encoding", "br")

		keyGzip := cache.GenerateKey(reqGzip, cfg, []string{"Accept-Encoding"})
		keyBr := cache.GenerateKey(reqBr, cfg, []string{"Accept-Encoding"})

		if keyGzip == keyBr {
			t.Errorf("differing Vary header values should produce different keys")
		}

		// Header already included in components should not duplicate in vary
		cfgHeader := config.CacheKeyConfig{
			Components: []string{"host", "path", "header.accept-encoding"},
		}
		keyHeader := cache.GenerateKey(reqGzip, cfgHeader, []string{"Accept-Encoding"})
		if keyHeader == "" {
			t.Fatal("expected non-empty key")
		}
	})

	t.Run("Invalid/unknown components ignored", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://example.com/page", nil)
		cfg := config.CacheKeyConfig{
			Components: []string{"host", "path", "unknown_comp"},
		}
		key := cache.GenerateKey(req, cfg, nil)
		if key == "" {
			t.Fatal("expected non-empty key")
		}
	})
}
