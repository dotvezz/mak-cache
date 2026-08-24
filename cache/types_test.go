package cache

import (
	"bytes"
	"math"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func dyn(s string) string {
	return string([]byte(s))
}

func dynSlice(ss []string) []string {
	res := make([]string, len(ss))
	for i := range ss {
		res[i] = dyn(ss[i])
	}
	return res
}

func measureHeapAllocation(n int, factory func() *Entry) uint64 {
	runtime.GC()
	var ms1 runtime.MemStats
	runtime.ReadMemStats(&ms1)

	slice := make([]*Entry, n)
	for i := 0; i < n; i++ {
		slice[i] = factory()
	}

	var ms2 runtime.MemStats
	runtime.ReadMemStats(&ms2)

	_ = slice[len(slice)-1] // prevent compiler optimization

	if ms2.TotalAlloc > ms1.TotalAlloc {
		return (ms2.TotalAlloc - ms1.TotalAlloc) / uint64(n)
	}
	return 0
}

func TestHeapSizeAccuracy(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		factory   func() *Entry
	}{
		{
			name:      "Minimal / Empty Entry",
			threshold: 0.70,
			factory: func() *Entry {
				e := &Entry{
					Status:  204,
					Body:    nil,
					Headers: nil,
					Metadata: Metadata{
						Date:    time.Now(),
						Expires: time.Now().Add(time.Minute),
						Evict:   time.Now().Add(time.Minute),
					},
				}
				e.RefreshHeapSize()
				return e
			},
		},
		{
			name:      "Small CSS Asset (512B)",
			threshold: 0.25,
			factory: func() *Entry {
				e := &Entry{
					Status: 200,
					Body:   bytes.Repeat([]byte(dyn("body{color:#333;margin:0;}")), 18),
					Headers: [][2]string{
						{dyn("Content-Type"), dyn("text/css; charset=utf-8")},
						{dyn("Cache-Control"), dyn("public, max-age=86400")},
						{dyn("ETag"), dyn("\"css-v1.2.3-hash\"")},
					},
					Metadata: Metadata{
						ETag:         dyn("\"css-v1.2.3-hash\""),
						Vary:         dynSlice([]string{"accept-encoding"}),
						CacheControl: dynSlice([]string{"public", "max-age=86400"}),
						Date:         time.Now(),
						Expires:      time.Now().Add(24 * time.Hour),
						Evict:        time.Now().Add(24 * time.Hour),
					},
				}
				e.RefreshHeapSize()
				return e
			},
		},
		{
			name:      "Medium JSON API Response (4KB)",
			threshold: 0.15,
			factory: func() *Entry {
				headers := [][2]string{
					{dyn("Content-Type"), dyn("application/json; charset=utf-8")},
					{dyn("Cache-Control"), dyn("public, max-age=300, s-maxage=600")},
					{dyn("Vary"), dyn("Accept-Encoding, Accept-Language, X-Api-Version")},
					{dyn("X-RateLimit-Limit"), dyn("10000")},
					{dyn("X-RateLimit-Remaining"), dyn("9950")},
					{dyn("X-RateLimit-Reset"), dyn("1600000000")},
					{dyn("Server"), dyn("Caddy/v2.8.4")},
					{dyn("Set-Cookie"), dyn("session=abcdef1234567890; Path=/; Secure; HttpOnly")},
					{dyn("Set-Cookie"), dyn("pref_lang=en-US; Path=/")},
				}
				e := &Entry{
					Status:  200,
					Body:    bytes.Repeat([]byte(dyn(`{"id":101,"name":"widget","properties":["fast","secure","cacheable"]},`)), 60),
					Headers: headers,
					Metadata: Metadata{
						ETag:         dyn("\"W/api-v2-widget-101\""),
						Vary:         dynSlice([]string{"accept-encoding", "accept-language", "x-api-version"}),
						CacheControl: dynSlice([]string{"public", "max-age=300", "s-maxage=600"}),
						Linked:       dynSlice([]string{"/api/v2/widget/101/details", "/api/v2/widget/101/metrics"}),
						Date:         time.Now(),
						Expires:      time.Now().Add(5 * time.Minute),
						Evict:        time.Now().Add(10 * time.Minute),
					},
				}
				e.RefreshHeapSize()
				return e
			},
		},
		{
			name:      "Large Binary Image Payload (64KB)",
			threshold: 0.05,
			factory: func() *Entry {
				e := &Entry{
					Status: 200,
					Body:   bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, 8192),
					Headers: [][2]string{
						{dyn("Content-Type"), dyn("image/png")},
						{dyn("Content-Length"), dyn("65536")},
						{dyn("Cache-Control"), dyn("public, max-age=31536000, immutable")},
						{dyn("ETag"), dyn("\"img-sha256-9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\"")},
					},
					Metadata: Metadata{
						ETag:         dyn("\"img-sha256-9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\""),
						CacheControl: dynSlice([]string{"public", "max-age=31536000", "immutable"}),
						Date:         time.Now(),
						Expires:      time.Now().Add(365 * 24 * time.Hour),
						Evict:        time.Now().Add(365 * 24 * time.Hour),
					},
				}
				e.RefreshHeapSize()
				return e
			},
		},
		{
			name:      "Vary-Heavy Entry with Many Links",
			threshold: 0.15,
			factory: func() *Entry {
				vary := dynSlice([]string{
					"accept", "accept-encoding", "accept-language", "authorization",
					"cookie", "origin", "x-requested-with", "x-custom-tenant",
				})
				links := make([]string, 20)
				for i := range links {
					links[i] = dyn("/linked/resource/path/item/" + strconv.Itoa(i))
				}
				e := &Entry{
					Status: 200,
					Body:   bytes.Repeat([]byte(dyn("html-data-chunk-payload-")), 40),
					Headers: [][2]string{
						{dyn("Content-Type"), dyn("text/html; charset=utf-8")},
						{dyn("Vary"), dyn("Accept, Accept-Encoding, Accept-Language, Authorization, Cookie, Origin, X-Requested-With, X-Custom-Tenant")},
					},
					Metadata: Metadata{
						ETag:         dyn("\"heavy-vary-etag-v1\""),
						Vary:         vary,
						CacheControl: dynSlice([]string{"public", "max-age=60"}),
						Linked:       links,
						Date:         time.Now(),
						Expires:      time.Now().Add(time.Minute),
						Evict:        time.Now().Add(time.Minute),
					},
				}
				e.RefreshHeapSize()
				return e
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sample := tt.factory()
			estimated := float64(sample.HeapSize())

			const iterations = 5000
			actual := float64(measureHeapAllocation(iterations, tt.factory))

			t.Logf("[%s] Estimated HeapSize(): %.0f bytes, Actual measured TotalAlloc: %.0f bytes", tt.name, estimated, actual)

			if actual > 0 {
				diff := math.Abs(estimated - actual)
				ratio := diff / actual
				t.Logf("[%s] Absolute Difference: %.0f bytes (%.2f%%)", tt.name, diff, ratio*100)

				if ratio > tt.threshold {
					t.Errorf("[%s] HeapSize estimate %.0f bytes deviated by %.2f%% from actual measured heap alloc %.0f bytes (threshold: %.0f%%)",
						tt.name, estimated, ratio*100, actual, tt.threshold*100)
				}
			}
		})
	}
}

func TestEntry_GetHeader(t *testing.T) {
	entry := &Entry{
		Headers: [][2]string{
			{"Content-Type", "text/html"},
			{"Set-Cookie", "session=abc"},
			{"Set-Cookie", "theme=dark"},
		},
	}

	t.Run("single header match", func(t *testing.T) {
		vals := entry.GetHeader("content-type")
		if len(vals) != 1 || vals[0] != "text/html" {
			t.Errorf("GetHeader('content-type') = %v, want ['text/html']", vals)
		}
	})

	t.Run("multiple header matches", func(t *testing.T) {
		vals := entry.GetHeader("Set-Cookie")
		if len(vals) != 2 || vals[0] != "session=abc" || vals[1] != "theme=dark" {
			t.Errorf("GetHeader('Set-Cookie') = %v, want ['session=abc', 'theme=dark']", vals)
		}
	})

	t.Run("non-existent header", func(t *testing.T) {
		vals := entry.GetHeader("X-Missing")
		if len(vals) != 0 {
			t.Errorf("GetHeader('X-Missing') = %v, want empty", vals)
		}
	})
}

func TestEntry_FromResponse(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), buf, func(status int, header http.Header) bool { return true })
	rec.Header().Set("Content-Type", "application/json")
	rec.Header().Add("Vary", "Accept-Encoding")
	rec.WriteHeader(http.StatusOK)
	_, _ = rec.Write([]byte(`{"status":"ok"}`))

	entry := &Entry{}
	entry.FromResponse(rec)

	if entry.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", entry.Status)
	}
	if string(entry.Body) != `{"status":"ok"}` {
		t.Errorf("Body = %q, want '{\"status\":\"ok\"}'", string(entry.Body))
	}
	if len(entry.GetHeader("Content-Type")) != 1 || entry.GetHeader("Content-Type")[0] != "application/json" {
		t.Errorf("Content-Type header mismatch = %v", entry.GetHeader("Content-Type"))
	}
}

func TestMetadata_EvictAt(t *testing.T) {
	now := time.Now()
	meta := &Metadata{Evict: now}
	if meta.EvictAt() != now {
		t.Errorf("EvictAt() = %v, want %v", meta.EvictAt(), now)
	}
}

func TestSizeClasses(t *testing.T) {
	tests := []struct {
		size int
		want int
	}{
		{0, 0},
		{5, 8},
		{12, 16},
		{20, 24},
		{30, 32},
		{40, 48},
		{60, 64},
		{75, 80},
		{90, 96},
		{100, 112},
		{120, 128},
		{135, 144},
		{150, 160},
		{170, 176},
		{185, 192},
		{200, 208},
		{220, 224},
		{230, 240},
		{250, 256},
		{300, 320},
		{350, 384},
		{400, 448},
		{500, 512},
		{600, 640}, // default case alignment to 64 bytes
	}

	for _, tt := range tests {
		got := sizeClass(tt.size)
		if got != tt.want {
			t.Errorf("sizeClass(%d) = %d, want %d", tt.size, got, tt.want)
		}
	}
}
