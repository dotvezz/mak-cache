package mak

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	mock_origin "github.com/dotvezz/mak-cache/_test/mock-origin"
	"github.com/dotvezz/mak-cache/config"
	"github.com/dustin/go-humanize"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func pointerTo[T any](v T) *T {
	return &v
}

func TestMiddleware_ServeHTTP_ConcurrentMissCollapse(t *testing.T) {
	gotMiddleware := must(New(
		WithConfig(config.Config{
			Timing: config.TimingConfig{
				TTL: 10 * time.Second,
			},
			Storage: []config.StorageConfig{
				{
					Otter: &config.OtterConfig{
						MemoryLimit: must(humanize.ParseBytes("100MB")),
					},
				},
			},
		}),
	))
	mockOrigin := new(mock_origin.MockOrigin)
	handle := gotMiddleware(mockOrigin).ServeHTTP

	const numRequests = 10
	type result struct {
		status      int
		cacheStatus string
	}
	results := make([]result, numRequests)
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := http.Request{
				Method: http.MethodGet,
				URL:    must(url.Parse("/slow/500?t10=1")),
			}
			handle(rec, &req)
			resp := rec.Result()
			if resp.Body != nil {
				resp.Body.Close()
			}
			cs := resp.Header.Get("Cache-Status")
			results[idx] = result{
				status:      resp.StatusCode,
				cacheStatus: cs,
			}
		}(i)
	}

	wg.Wait()

	hasCollapsed := false
	for _, res := range results {
		if res.status != 200 {
			t.Errorf("HTTP Status: got %d, want 200", res.status)
		}
		if strings.Contains(res.cacheStatus, "collapsed") {
			hasCollapsed = true
		}
	}

	if !hasCollapsed {
		t.Errorf("Cache-Status: expected at least one response to contain 'collapsed'")
	}

	if mockOrigin.RequestCount() != 1 {
		t.Errorf("Origin Request Count: got %d, want 1", mockOrigin.RequestCount())
	}
}

func TestMiddleware_ServeHTTP(t *testing.T) {
	type want struct {
		status             int
		body               []byte
		cacheStatusNeeds   []string
		cacheStatusAvoids  []string
		originRequestCount *int
	}
	type test struct {
		name       string
		req        http.Request
		delay      time.Duration
		resetCount bool
		want       want
	}
	tests := map[string][]test{
		"simple example": {
			{
				name: "inital request miss",
				req: http.Request{
					Method: "GET",
					URL:    &url.URL{Path: "/cacheable"},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"miss"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "second request hit",
				req: http.Request{
					Method: "GET",
					URL:    &url.URL{Path: "/cacheable"},
				},
				resetCount: true,
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit", "ttl=10", "key=1da3b6a6967c85e0e85ccb735a3bb77d"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"bypasses": {
			{
				name: "post bypasses cache",
				req: http.Request{
					Method: http.MethodPost,
					URL:    &url.URL{Path: "/thing"},
				},
				resetCount: true,
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=method"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "put bypasses cache",
				req: http.Request{
					Method: http.MethodPut,
					URL:    &url.URL{Path: "/thing"},
				},
				resetCount: true,
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=method"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "patch bypasses cache",
				req: http.Request{
					Method: http.MethodPatch,
					URL:    &url.URL{Path: "/thing"},
				},
				resetCount: true,
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=method"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Basic Miss and Hit": {
			{
				name: "miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t1=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t1=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"TTL Expiration": {
			{
				name: "initial miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t2=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "expiration after TTL",
				delay: 11 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t2=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: no-store": {
			{
				name: "first request miss not stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/no-store?t3=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					cacheStatusAvoids:  []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "second request not hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/no-store?t3=1")),
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: private": {
			{
				name: "first request miss not stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/private?t4=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					cacheStatusAvoids:  []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "second request not hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/private?t4=1")),
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: max-age Override": {
			{
				name: "miss and stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t5=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "immediate hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t5=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "expired after max-age",
				delay: 3 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t5=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Vary: Accept-Encoding": {
			{
				name: "gzip miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/vary/accept-encoding?t6=1")),
					Header: http.Header{"Accept-Encoding": []string{"gzip"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "gzip hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/vary/accept-encoding?t6=1")),
					Header: http.Header{"Accept-Encoding": []string{"gzip"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "br vary miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/vary/accept-encoding?t6=1")),
					Header: http.Header{"Accept-Encoding": []string{"br"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(2),
				},
			},
			{
				name: "br hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/vary/accept-encoding?t6=1")),
					Header: http.Header{"Accept-Encoding": []string{"br"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Vary: *": {
			{
				name: "first request miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/vary/star?t7=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "second request bypass",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/vary/star?t7=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=bypass"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"ETag Conditional": {
			{
				name: "first request miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/etag?t8=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "conditional request matching ETag 304",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/etag?t8=1")),
					Header: http.Header{"If-None-Match": []string{`"test-etag-12345"`}},
				},
				want: want{
					status:             304,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "conditional request mismatch ETag 200",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/etag?t8=1")),
					Header: http.Header{"If-None-Match": []string{`"mismatch-etag"`}},
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Non-Cacheable HTTP Methods": {
			{
				name: "post fwd method",
				req: http.Request{
					Method: http.MethodPost,
					URL:    must(url.Parse("/thing")),
				},
				resetCount: true,
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=method"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "put fwd method",
				req: http.Request{
					Method: http.MethodPut,
					URL:    must(url.Parse("/thing")),
				},
				resetCount: true,
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=method"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "delete fwd method",
				req: http.Request{
					Method: http.MethodDelete,
					URL:    must(url.Parse("/cacheable")),
				},
				resetCount: true,
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=method"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Stale-While-Revalidate": {
			{
				name: "initial miss and stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/swr/10?t11=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "stale window request triggers background refresh",
				delay: 2 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/swr/10?t11=1")),
				},
				want: want{
					status: 200,
				},
			},
			{
				name:  "hit after background refresh completes",
				delay: 1 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/swr/10?t11=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: s-maxage": {
			{
				name: "miss and stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/s-maxage?t12=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "immediate hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/s-maxage?t12=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "expired after s-maxage",
				delay: 3 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/s-maxage?t12=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: no-cache Response Directive": {
			{
				name: "first request miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/no-cache?t13=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "second request revalidates or not unvalidated hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/no-cache?t13=1")),
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: no-cache Request Directive": {
			{
				name: "store in cache",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t14=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "normal request gets hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t14=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "request with no-cache forces origin revalidation",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t14=1")),
					Header: http.Header{"Cache-Control": []string{"no-cache"}},
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: no-store Request Directive": {
			{
				name: "request with no-store in request header not stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t15=1")),
					Header: http.Header{"Cache-Control": []string{"no-store"}},
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "subsequent request is miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t15=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Expires Header (Past vs Future)": {
			{
				name: "expires past initial request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/expires/past?t16a=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "expires past second request not a fresh hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/expires/past?t16a=1")),
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
			{
				name: "expires future initial request miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/expires/future?t16b=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					originRequestCount: pointerTo(3),
				},
			},
			{
				name: "expires future second request hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/expires/future?t16b=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(3),
				},
			},
		},
		"max-age Precedence over Expires": {
			{
				name: "miss and stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/expires/max-age-override?t17=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "immediate hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/expires/max-age-override?t17=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "expired after max-age",
				delay: 3 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/expires/max-age-override?t17=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: must-revalidate Directive": {
			{
				name: "stored on miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/must-revalidate?t18=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "hit while fresh",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/must-revalidate?t18=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "requires revalidation when stale",
				delay: 3 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/must-revalidate?t18=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Weak ETag Conditional Requests": {
			{
				name: "initial request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/etag/weak?t19=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "conditional request with weak ETag 304",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/etag/weak?t19=1")),
					Header: http.Header{"If-None-Match": []string{`W/"weak-etag-1234"`}},
				},
				want: want{
					status:             304,
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Cache Invalidation on Unsafe Methods": {
			{
				name: "prime cache",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/thing?t20=1")),
				},
				want: want{
					status:             404,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "verify hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/thing?t20=1")),
				},
				want: want{
					status:             404,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "send unsafe method POST to target URI",
				req: http.Request{
					Method: http.MethodPost,
					URL:    must(url.Parse("/thing?t20=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=method"},
					originRequestCount: pointerTo(2),
				},
			},
			{
				name: "subsequent GET is cache miss after POST invalidation",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/thing?t20=1")),
				},
				want: want{
					status:             404,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(3),
				},
			},
		},
		"Uncacheable Status Code 500": {
			{
				name: "miss on first request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/status/500?t21=1")),
				},
				want: want{
					status:             500,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "status 500 is not cached",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/status/500?t21=1")),
				},
				want: want{
					status:             500,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cacheable Status Code 404": {
			{
				name: "stored on miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/status/404?t22=1")),
				},
				want: want{
					status:             404,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "hit on second request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/status/404?t22=1")),
				},
				want: want{
					status:             404,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Age Header Presence on Hits": {
			{
				name: "first request miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t23=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "second request hit after delay",
				delay: 1 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t23=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"HEAD Method Caching": {
			{
				name: "prime cache with GET",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t24=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "HEAD request for cached resource",
				req: http.Request{
					Method: http.MethodHead,
					URL:    must(url.Parse("/cacheable?t24=1")),
				},
				want: want{
					status: 200,
				},
			},
		},
		"Cache-Control: max-age=0 Request Directive": {
			{
				name: "prime cache",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t25=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "max-age=0 request forces revalidation",
				delay: 1 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t25=1")),
					Header: http.Header{"Cache-Control": []string{"max-age=0"}},
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: public Directive": {
			{
				name: "miss and stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/public?t26=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "cache hit",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/public?t26=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Authorized Request Default": {
			{
				name: "authorized request miss not stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t27=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss"},
					cacheStatusAvoids:  []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "authorized request second request miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cacheable?t27=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Authorized Request with public Directive": {
			{
				name: "stored when public directive present",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/public?t28=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "cache hit on second request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/public?t28=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Authorized Request with s-maxage Directive": {
			{
				name: "stored when s-maxage directive present",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/s-maxage?t29=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "cache hit on second request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/s-maxage?t29=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Authorized Request with must-revalidate Directive": {
			{
				name: "stored when must-revalidate directive present",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/must-revalidate?t30=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "cache hit on second request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/must-revalidate?t30=1")),
					Header: http.Header{"Authorization": []string{"Bearer secret-token-123"}},
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"hit"},
					originRequestCount: pointerTo(1),
				},
			},
		},
		"Cache-Control: min-fresh Request Directive": {
			{
				name: "stored on miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t31=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "revalidates or miss when remaining freshness < min-fresh",
				delay: 1 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t31=1")),
					Header: http.Header{"Cache-Control": []string{"min-fresh=5"}},
				},
				want: want{
					status:             200,
					cacheStatusAvoids:  []string{"hit"},
					originRequestCount: pointerTo(2),
				},
			},
		},
		"Cache-Control: max-stale Request Directive": {
			{
				name: "stored on miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t32=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "serves stale response when max-stale permits",
				delay: 3 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t32=1")),
					Header: http.Header{"Cache-Control": []string{"max-stale=10"}},
				},
				want: want{
					status: 200,
				},
			},
		},
		"If-Modified-Since Conditional Request": {
			{
				name: "initial request",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/last-modified?t33=1")),
				},
				want: want{
					status:             200,
					originRequestCount: pointerTo(1),
				},
			},
			{
				name: "conditional request with matching Last-Modified",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/last-modified?t33=1")),
					Header: http.Header{"If-Modified-Since": []string{"Wed, 21 Oct 2015 07:28:00 GMT"}},
				},
				want: want{
					status: 200,
				},
			},
		},
		"Stale-While-Revalidate Window Expiration": {
			{
				name: "initial miss and stored",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/swr/10?t34=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd=uri-miss", "stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "forwards to origin after SWR window expires",
				delay: 12 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/swr/10?t34=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"fwd="},
					originRequestCount: pointerTo(2),
				},
			},
		},
		// TODO: Cache-Control: stale-if-error _Response_ Directive
		"Cache-Control: stale-if-error Request Directive": {
			{
				name: "stored on miss",
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t36=1")),
				},
				want: want{
					status:             200,
					cacheStatusNeeds:   []string{"stored"},
					originRequestCount: pointerTo(1),
				},
			},
			{
				name:  "header accepted and response returned",
				delay: 3 * time.Second,
				req: http.Request{
					Method: http.MethodGet,
					URL:    must(url.Parse("/cache-control/max-age/2?t36=1")),
					Header: http.Header{"Cache-Control": []string{"stale-if-error=10"}},
				},
				want: want{
					status: 200,
				},
			},
		},
	}

	for group, ts := range tests {
		mockTime := time.Now()
		now = func() time.Time {
			return mockTime
		}
		defer func() {
			now = time.Now
		}()

		gotMiddleware := must(New(
			WithConfig(config.Config{
				Timing: config.TimingConfig{
					TTL: 10 * time.Second,
				},
				Storage: []config.StorageConfig{
					{
						Otter: &config.OtterConfig{
							MemoryLimit: must(humanize.ParseBytes("100MB")),
						},
					},
				},
			}),
		))
		mockOrigin := new(mock_origin.MockOrigin)
		handle := gotMiddleware(mockOrigin).ServeHTTP

		// always reset the counter at the beginning of each test group
		mockOrigin.ResetCount()
		for _, tt := range ts {
			t.Run(fmt.Sprintf("%s %s", group, tt.name), func(t *testing.T) {
				if tt.delay > 0 {
					mockTime = mockTime.Add(tt.delay)
					// Small yield to allow background goroutines (e.g. SWR background refresh) to execute
					time.Sleep(10 * time.Millisecond)
				}
				rec := httptest.NewRecorder()

				handle(rec, &tt.req)

				resp := rec.Result()
				var gotBody []byte
				if resp.Body != nil {
					defer resp.Body.Close()
					gotBody, _ = io.ReadAll(resp.Body)
				}

				if tt.want.status != 0 && resp.StatusCode != tt.want.status {
					t.Errorf("HTTP Status: got %d, want %d", resp.StatusCode, tt.want.status)
				}

				if tt.want.body != nil && !slices.Equal(gotBody, tt.want.body) {
					t.Errorf("HTTP Body:\n  got %s\n want %s", string(gotBody), string(tt.want.body))
				}

				cacheStatus := resp.Header.Values("Cache-Status")
				noCacheStatus := len(cacheStatus) == 0
				wantCacheStatus := len(tt.want.cacheStatusNeeds) > 0

				if noCacheStatus && wantCacheStatus {
					t.Errorf("Cache-Status: header is missing")
				}

				var gotCacheStatus string
				if !noCacheStatus {
					gotCacheStatus = cacheStatus[len(cacheStatus)-1]
				}

				for _, need := range tt.want.cacheStatusNeeds {
					if !strings.Contains(gotCacheStatus, need) {
						t.Errorf("Cache-Status: missing %s, got %s", need, gotCacheStatus)
					}
				}

				for _, avoid := range tt.want.cacheStatusAvoids {
					if strings.Contains(gotCacheStatus, avoid) {
						t.Errorf("Cache-Status: should not contain %s, got %s", avoid, gotCacheStatus)
					}
				}

				if tt.want.originRequestCount != nil && *tt.want.originRequestCount != mockOrigin.RequestCount() {
					t.Errorf("Origin Request Count: got %d, want %d", mockOrigin.RequestCount(), *tt.want.originRequestCount)
				}

				if tt.resetCount {
					mockOrigin.ResetCount()
				}
			})
		}
	}
}
