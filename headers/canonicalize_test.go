package headers

import (
	"net/http"
	"testing"
)

func TestCanonicalizeRequest(t *testing.T) {
	t.Run("canonicalizes Accept and Accept-Language headers", func(t *testing.T) {
		hs := http.Header{
			"Accept":          []string{"text/html", "application/xhtml+xml"},
			"Accept-Language": []string{"en-US", "en"},
			"X-Custom":        []string{"value1", "value2"},
		}

		CanonicalizeRequest(hs)

		if got := hs.Get("Accept"); got != "application/xhtml+xml, text/html" {
			t.Errorf("Accept = %q, want 'application/xhtml+xml, text/html'", got)
		}
		if got := hs.Get("Accept-Language"); got != "en, en-us" {
			t.Errorf("Accept-Language = %q, want 'en, en-us'", got)
		}
		if got := hs.Get("X-Custom"); got != "value1" {
			t.Errorf("X-Custom modified unexpectedly = %q", got)
		}
	})

	t.Run("canonicalizes Accept-Encoding header", func(t *testing.T) {
		hs := http.Header{
			"Accept-Encoding": []string{"gzip", "br", "zstd"},
		}

		CanonicalizeRequest(hs)

		if got := hs.Get("Accept-Encoding"); got != "zstd, br, gzip" {
			t.Errorf("Accept-Encoding = %q, want 'zstd, br, gzip'", got)
		}
	})
}
