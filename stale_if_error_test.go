package mak

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dotvezz/mak-cache/config"
	"github.com/dustin/go-humanize"
)

func TestIsStaleEligibleErrorStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
		{http.StatusOK, false},
		{http.StatusNoContent, false},
		{http.StatusNotModified, false},
		{http.StatusBadRequest, false},
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, false},
		{http.StatusNotImplemented, false},
		{http.StatusHTTPVersionNotSupported, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.status), func(t *testing.T) {
			if got := isStaleEligibleErrorStatus(tt.status); got != tt.want {
				t.Errorf("isStaleEligibleErrorStatus(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func newStaleIfErrorTestMiddleware(t *testing.T) func(next http.Handler) http.Handler {
	t.Helper()
	mw, err := New(
		WithConfig(config.Config{
			Timing: config.TimingConfig{
				TTL: 10 * time.Second,
			},
			Refresh: config.RefreshConfig{
				Disable: true,
			},
			Storage: []config.StorageConfig{
				{
					Otter: &config.OtterConfig{
						MemoryLimit: must(humanize.ParseBytes("100MB")),
					},
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}
	return mw
}

func TestStaleIfError_ResponseDirective_EligibleErrors(t *testing.T) {
	errorStatuses := []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	for _, errStatus := range errorStatuses {
		t.Run(fmt.Sprintf("upstream_%d", errStatus), func(t *testing.T) {
			mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
			now = func() time.Time { return mockTime }
			defer func() { now = time.Now }()

			var calls atomic.Int32
			origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Cache-Control", "max-age=2, stale-if-error=60")
					w.Header().Set("Content-Type", "text/plain")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("cached-body"))
					return
				}
				w.WriteHeader(errStatus)
				_, _ = w.Write([]byte("upstream-error"))
			})

			handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

			// 1. Initial request primes cache
			rec1 := httptest.NewRecorder()
			req1 := &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))}
			handle(rec1, req1)

			resp1 := rec1.Result()
			body1, _ := io.ReadAll(resp1.Body)
			_ = resp1.Body.Close()
			if resp1.StatusCode != http.StatusOK || string(body1) != "cached-body" {
				t.Fatalf("initial request: got status %d body %q, want 200 %q", resp1.StatusCode, string(body1), "cached-body")
			}

			// 2. Advance past max-age (2s) but within stale-if-error (60s)
			mockTime = mockTime.Add(5 * time.Second)

			rec2 := httptest.NewRecorder()
			req2 := &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))}
			handle(rec2, req2)

			resp2 := rec2.Result()
			body2, _ := io.ReadAll(resp2.Body)
			_ = resp2.Body.Close()

			if resp2.StatusCode != http.StatusOK {
				t.Errorf("stale-if-error response status: got %d, want %d", resp2.StatusCode, http.StatusOK)
			}
			if string(body2) != "cached-body" {
				t.Errorf("stale-if-error response body: got %q, want %q", string(body2), "cached-body")
			}
			if calls.Load() != 2 {
				t.Errorf("origin calls: got %d, want 2", calls.Load())
			}
		})
	}
}

func TestStaleIfError_ResponseDirective_NonEligibleError(t *testing.T) {
	nonEligibleStatuses := []int{
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusNotImplemented,
	}

	for _, errStatus := range nonEligibleStatuses {
		t.Run(fmt.Sprintf("upstream_%d", errStatus), func(t *testing.T) {
			mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
			now = func() time.Time { return mockTime }
			defer func() { now = time.Now }()

			var calls atomic.Int32
			origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Cache-Control", "max-age=2, stale-if-error=60")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("cached-body"))
					return
				}
				w.WriteHeader(errStatus)
				_, _ = w.Write([]byte("non-eligible-error"))
			})

			handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

			// 1. Prime cache
			rec1 := httptest.NewRecorder()
			handle(rec1, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
			_ = rec1.Result().Body.Close()

			// 2. Advance past max-age
			mockTime = mockTime.Add(5 * time.Second)

			rec2 := httptest.NewRecorder()
			handle(rec2, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})

			resp2 := rec2.Result()
			body2, _ := io.ReadAll(resp2.Body)
			_ = resp2.Body.Close()

			if resp2.StatusCode != errStatus {
				t.Errorf("status: got %d, want %d", resp2.StatusCode, errStatus)
			}
			if string(body2) != "non-eligible-error" {
				t.Errorf("body: got %q, want %q", string(body2), "non-eligible-error")
			}
		})
	}
}

func TestStaleIfError_ResponseDirective_WindowExpired(t *testing.T) {
	mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
	now = func() time.Time { return mockTime }
	defer func() { now = time.Now }()

	var calls atomic.Int32
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Cache-Control", "max-age=2, stale-if-error=10")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("cached-body"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream-error"))
	})

	handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

	// 1. Prime cache
	rec1 := httptest.NewRecorder()
	handle(rec1, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
	_ = rec1.Result().Body.Close()

	// 2. Advance past max-age + stale-if-error (2s + 10s = 12s)
	mockTime = mockTime.Add(15 * time.Second)

	rec2 := httptest.NewRecorder()
	handle(rec2, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})

	resp2 := rec2.Result()
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusInternalServerError {
		t.Errorf("status after window expired: got %d, want %d", resp2.StatusCode, http.StatusInternalServerError)
	}
	if string(body2) != "upstream-error" {
		t.Errorf("body after window expired: got %q, want %q", string(body2), "upstream-error")
	}
}

func TestStaleIfError_ResponseDirective_WindowMeasuredFromStaleness(t *testing.T) {
	// Per RFC 5861 Section 4, stale-if-error specifies the number of seconds
	// after the response has become stale that it may still be used on error.
	mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
	now = func() time.Time { return mockTime }
	defer func() { now = time.Now }()

	var calls atomic.Int32
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Cache-Control", "max-age=10, stale-if-error=5")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("cached-body"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream-error"))
	})

	handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

	// 1. Prime cache at t=0 (expires at t=10, stale-if-error until t=15)
	rec1 := httptest.NewRecorder()
	handle(rec1, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
	_ = rec1.Result().Body.Close()

	// 2. Advance to t=12 (2s stale, within 5s stale-if-error window)
	mockTime = mockTime.Add(12 * time.Second)

	rec2 := httptest.NewRecorder()
	handle(rec2, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})

	resp2 := rec2.Result()
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp2.StatusCode, http.StatusOK)
	}
	if string(body2) != "cached-body" {
		t.Errorf("body: got %q, want %q", string(body2), "cached-body")
	}
}

func TestStaleIfError_RequestDirective_EligibleErrors(t *testing.T) {
	errorStatuses := []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	for _, errStatus := range errorStatuses {
		t.Run(fmt.Sprintf("upstream_%d", errStatus), func(t *testing.T) {
			mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
			now = func() time.Time { return mockTime }
			defer func() { now = time.Now }()

			var calls atomic.Int32
			origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Cache-Control", "max-age=2")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("cached-body"))
					return
				}
				w.WriteHeader(errStatus)
				_, _ = w.Write([]byte("upstream-error"))
			})

			handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

			// 1. Prime cache (no stale-if-error on response)
			rec1 := httptest.NewRecorder()
			handle(rec1, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
			_ = rec1.Result().Body.Close()

			// 2. Advance past max-age
			mockTime = mockTime.Add(5 * time.Second)

			rec2 := httptest.NewRecorder()
			req2 := &http.Request{
				Method: http.MethodGet,
				URL:    must(url.Parse("/resource")),
				Header: http.Header{"Cache-Control": []string{"stale-if-error=60"}},
			}
			handle(rec2, req2)

			resp2 := rec2.Result()
			body2, _ := io.ReadAll(resp2.Body)
			_ = resp2.Body.Close()

			if resp2.StatusCode != http.StatusOK {
				t.Errorf("status: got %d, want %d", resp2.StatusCode, http.StatusOK)
			}
			if string(body2) != "cached-body" {
				t.Errorf("body: got %q, want %q", string(body2), "cached-body")
			}
			if calls.Load() != 2 {
				t.Errorf("origin calls: got %d, want 2", calls.Load())
			}
		})
	}
}

func TestStaleIfError_RequestDirective_WindowExpired(t *testing.T) {
	mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
	now = func() time.Time { return mockTime }
	defer func() { now = time.Now }()

	var calls atomic.Int32
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Cache-Control", "max-age=2")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("cached-body"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream-error"))
	})

	handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

	// 1. Prime cache
	rec1 := httptest.NewRecorder()
	handle(rec1, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
	_ = rec1.Result().Body.Close()

	// 2. Advance past max-age + request stale-if-error (2s + 2s = 4s)
	mockTime = mockTime.Add(10 * time.Second)

	rec2 := httptest.NewRecorder()
	req2 := &http.Request{
		Method: http.MethodGet,
		URL:    must(url.Parse("/resource")),
		Header: http.Header{"Cache-Control": []string{"stale-if-error=2"}},
	}
	handle(rec2, req2)

	resp2 := rec2.Result()
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", resp2.StatusCode, http.StatusInternalServerError)
	}
	if string(body2) != "upstream-error" {
		t.Errorf("body: got %q, want %q", string(body2), "upstream-error")
	}
}

func TestStaleIfError_RequestDirective_WindowMeasuredFromStaleness(t *testing.T) {
	mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
	now = func() time.Time { return mockTime }
	defer func() { now = time.Now }()

	var calls atomic.Int32
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Cache-Control", "max-age=10")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("cached-body"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("upstream-error"))
	})

	handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

	// 1. Prime cache at t=0 (expires at t=10)
	rec1 := httptest.NewRecorder()
	handle(rec1, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
	_ = rec1.Result().Body.Close()

	// 2. Advance to t=12 (2s stale, within request stale-if-error=5)
	mockTime = mockTime.Add(12 * time.Second)

	rec2 := httptest.NewRecorder()
	req2 := &http.Request{
		Method: http.MethodGet,
		URL:    must(url.Parse("/resource")),
		Header: http.Header{"Cache-Control": []string{"stale-if-error=5"}},
	}
	handle(rec2, req2)

	resp2 := rec2.Result()
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp2.StatusCode, http.StatusOK)
	}
	if string(body2) != "cached-body" {
		t.Errorf("body: got %q, want %q", string(body2), "cached-body")
	}
}

func TestStaleIfError_NoCachedEntry(t *testing.T) {
	mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
	now = func() time.Time { return mockTime }
	defer func() { now = time.Now }()

	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream-error"))
	})

	handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

	rec := httptest.NewRecorder()
	req := &http.Request{
		Method: http.MethodGet,
		URL:    must(url.Parse("/missing")),
		Header: http.Header{"Cache-Control": []string{"stale-if-error=60"}},
	}
	handle(rec, req)

	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if string(body) != "upstream-error" {
		t.Errorf("body: got %q, want %q", string(body), "upstream-error")
	}
}

func TestStaleIfError_OriginRecoversAfterError(t *testing.T) {
	mockTime := must(time.Parse(time.RFC3339, "2023-01-01T00:00:00Z"))
	now = func() time.Time { return mockTime }
	defer func() { now = time.Now }()

	var calls atomic.Int32
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.Header().Set("Cache-Control", "max-age=2, stale-if-error=60")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("v1"))
		case 2:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad-gateway"))
		default:
			w.Header().Set("Cache-Control", "max-age=10")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("v2"))
		}
	})

	handle := newStaleIfErrorTestMiddleware(t)(origin).ServeHTTP

	// Fill the cache with a valid value
	rec1 := httptest.NewRecorder()
	handle(rec1, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
	_ = rec1.Result().Body.Close()

	// Origin 502, serve v1
	mockTime = mockTime.Add(5 * time.Second)
	rec2 := httptest.NewRecorder()
	handle(rec2, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
	resp2 := rec2.Result()
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK || string(body2) != "v1" {
		t.Errorf("on error: got status %d body %q, want 200 %q", resp2.StatusCode, string(body2), "v1")
	}

	// Origin recovers, serve fresh v2
	rec3 := httptest.NewRecorder()
	handle(rec3, &http.Request{Method: http.MethodGet, URL: must(url.Parse("/resource"))})
	resp3 := rec3.Result()
	body3, _ := io.ReadAll(resp3.Body)
	_ = resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK || string(body3) != "v2" {
		t.Errorf("on recovery: got status %d body %q, want 200 %q", resp3.StatusCode, string(body3), "v2")
	}
}
