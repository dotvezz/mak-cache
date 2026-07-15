package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/dotvezz/caddy-cache/headers"
	"golang.org/x/sync/singleflight"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/storage"
)

var errNotBuffered = errors.New("not buffered")

type Handler struct {
	config.Config

	ConfigKey string `json:"config_key"`

	metadataStorage storage.Provider[*cache.Metadata]
	entryStorage    storage.Provider[*cache.Entry]

	singleflight *singleflight.Group

	now func() time.Time
}

const ContextKeyRequestTime = "caddy_cache-request_time"
const ContextKeyCacheStatus = "caddy_cache-cache_status"
const ContextKeyOriginalRequest = "caddy_cache-original_request"

func withRequestTime(ctx context.Context, time time.Time) context.Context {
	return context.WithValue(ctx, ContextKeyRequestTime, time)
}

func getRequestTime(ctx context.Context) time.Time {
	if t, ok := ctx.Value(ContextKeyRequestTime).(time.Time); ok {
		return t
	}

	return time.Time{}
}

func withOriginalRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, ContextKeyOriginalRequest, r)
}

func getOriginalRequest(ctx context.Context) *http.Request {
	if r, ok := ctx.Value(ContextKeyOriginalRequest).(*http.Request); ok {
		return r
	}
	return nil
}

func withCacheStatus(ctx context.Context, status *headers.CacheStatus) context.Context {
	return context.WithValue(ctx, ContextKeyCacheStatus, status)
}

func getCacheStatus(ctx context.Context) *headers.CacheStatus {
	if cs, ok := ctx.Value(ContextKeyCacheStatus).(*headers.CacheStatus); ok {
		return cs
	}

	return &headers.CacheStatus{}
}

func (h *Handler) getMetadata(ctx context.Context, key string) (*cache.Metadata, error) {
	if h.metadataStorage == nil {
		e, err := h.entryStorage.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		return &e.Metadata, err
	}

	return h.metadataStorage.Get(ctx, key)
}

func (h *Handler) setMetadata(ctx context.Context, key string, meta *cache.Metadata) error {
	if meta == nil {
		return fmt.Errorf("metadata is nil")
	}

	if h.metadataStorage == nil {
		e := &cache.Entry{
			Metadata: *meta,
		}
		return h.entryStorage.Set(ctx, key, e)
	}

	return h.metadataStorage.Set(ctx, key, meta)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) (err error) {
	var (
		meta  *cache.Metadata
		entry *cache.Entry
	)

	headers.CanonicalizeRequest(r.Header)

	ctx := r.Context()
	cacheStatus := headers.CacheStatus{
		Key: cache.GenerateKey(r, h.Key, nil),
	}
	ctx = withCacheStatus(ctx, &cacheStatus)
	ctx = withRequestTime(ctx, h.now())
	ctx = withOriginalRequest(ctx, r.Clone(r.Context()))
	r = r.WithContext(ctx)

	if r.Method != http.MethodHead && r.Method != http.MethodGet {
		cacheStatus.FwdMethod = true
		return h.bypass(w, r, next)
	}

	meta, err = h.getMetadata(r.Context(), cacheStatus.Key)

	if err != nil {
		cacheStatus.FwdURIMiss = true
		return h.forward(w, r, next)
	}

	if slices.Contains(meta.Vary, "*") || slices.Contains(meta.CacheControl, "No-Store") {
		cacheStatus.FwdBypass = true
		return h.bypass(w, r, next)
	}

	if len(meta.Vary) > 0 {
		cacheStatus.Key = cache.GenerateKey(r, h.Key, meta.Vary)
	}

	entry, err = h.entryStorage.Get(ctx, cacheStatus.Key)
	if err != nil {
		if len(meta.Vary) > 0 {
			cacheStatus.FwdVaryMiss = true
		}
		return h.forward(w, r, next)
	}

	if entry.Expires.Before(h.now()) {
		if !h.Refresh.Disable && entry.Expires.Before(h.now().Add(time.Duration(h.Timing.MaxStale))) {
			h.backgroundRefresh(r, next)
		} else {
			cacheStatus.FwdStale = true
			return h.forward(w, r, next)
		}
	}

	{
		ifNoneMatchHeader := r.Header["If-None-Match"]
		ifNoneMatchSet := make([]string, 0)
		for i := range ifNoneMatchHeader {
			for _, v := range strings.Split(ifNoneMatchHeader[i], ",") {
				ifNoneMatchSet = append(ifNoneMatchSet, strings.TrimSpace(v))
			}
		}

		if len(ifNoneMatchSet) > 0 && !slices.Contains(ifNoneMatchSet, entry.ETag) {
			return h.hit(w, r, entry)
		} else if len(ifNoneMatchSet) == 0 {
			cacheStatus.Hit = true
		} else if len(ifNoneMatchSet) > 0 && slices.Contains(ifNoneMatchSet, entry.ETag) {
			return h.notModified(w, r, entry)
		}
	}

	return h.hit(w, r, entry)

}

func (h *Handler) shouldBuffer(status int, _ http.Header) bool {
	// TODO: Handle negative caching
	return status >= 200 && status < 300
}

func (h *Handler) notModified(w http.ResponseWriter, r *http.Request, e *cache.Entry) error {
	h.hitHeaders(w, r, e)
	w.WriteHeader(http.StatusNotModified)
	return nil
}

func (h *Handler) hitHeaders(w http.ResponseWriter, r *http.Request, e *cache.Entry) {
	hs := w.Header()
	for i := range e.Headers {
		hs.Add(e.Headers[i][0], e.Headers[i][1])
	}

	ttl := e.Expires.Sub(h.now()) + time.Duration(rand.IntN(int(h.Timing.TTLSplay)))
	{
		cacheStatus := getCacheStatus(r.Context())
		cacheStatus.TTL = ttl
		cacheStatus.Hit = true
		hs.Add("Cache-Status", cacheStatus.String())
	}

	{
		expires := headers.Expires(getRequestTime(r.Context()).Add(ttl))
		hs.Add("Expires", expires.String())
	}

	{
		age := headers.Age(getRequestTime(r.Context()).Sub(e.Date))
		hs.Add("Age", age.String())
	}

	if e.ETag != "" {
		hs.Set("ETag", e.ETag)
	}
}

func (h *Handler) hit(w http.ResponseWriter, r *http.Request, e *cache.Entry) error {
	h.hitHeaders(w, r, e)
	w.WriteHeader(e.Status)
	_, err := w.Write(e.Body)
	return err
}

func (h *Handler) bypass(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	rec := caddyhttp.NewResponseRecorder(w, buf, h.shouldBuffer)
	err := next.ServeHTTP(rec, r)
	if err != nil {
		return err
	}

	cacheStatus := getCacheStatus(r.Context())
	rec.Header().Add("Cache-Status", cacheStatus.String())

	w.WriteHeader(rec.Status())

	if !rec.Buffered() {
		return nil
	}

	_, err = buf.WriteTo(w)
	return err
}
