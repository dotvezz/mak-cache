package cache

import (
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"slices"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/requests"
	"golang.org/x/sync/singleflight"

	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/headers"
	"github.com/dotvezz/caddy-cache/storage"
)

var errNotBuffered = errors.New("not buffered")

type Handler struct {
	*slog.Logger
	config.Config

	ConfigKey       string `json:"config_key"`
	metadataStorage storage.Provider[*cache.Metadata]

	entryStorage storage.Provider[*cache.Entry]

	singleflight *singleflight.Group

	now func() time.Time
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) (err error) {
	cacheStatus := new(headers.CacheStatus)
	requestTime := h.now()

	reqCC := headers.CacheControl{}
	{
		cc := r.Header.Values("Cache-Control")
		if len(cc) > 0 {
			err = reqCC.FromString(cc[len(cc)-1])
			if err != nil {
				h.Info("Request Cache-Control",
					slog.String("error", err.Error()),
					slog.String("Cache-Control", cc[len(cc)-1]),
				)
				// If there was some error parsing Cache-Control, we'll just ignore/strip it and continue
				reqCC = headers.CacheControl{}
				r.Header.Del("Cache-Control")
			}
		}
	}

	if !reqCC.Cacheable(false) {
		cacheStatus.FwdBypass = true
		w.Header().Add("Cache-Status", cacheStatus.String())
		return next.ServeHTTP(w, r)
	}

	headers.CanonicalizeRequest(r.Header)
	cacheStatus.Key = cache.GenerateKey(r, h.Key, nil)

	meta, found := h.getMetadata(r.Context(), cacheStatus.Key)

	if !requests.IsSafeMethod(r.Method) {
		cacheStatus.FwdMethod = true
		// Found metadata, but forwarded because of an unsafe method, so we need to invalidate
		// RFC 9211 Section 4.4
		return h.handleAndInvalidate(w, r, cacheStatus, next)
	} else if !found {
		// Miss
		cacheStatus.FwdURIMiss = true
		return h.forward(w, r, cacheStatus, requestTime, next)
	}

	respCC := headers.CacheControl{}
	err = respCC.FromDirectives(meta.CacheControl)
	if err != nil {
		h.Info("Cache-Control", slog.String("error", err.Error()))
	}

	if slices.Contains(meta.Vary, "*") || !respCC.Cacheable(true) {
		cacheStatus.FwdBypass = true
		w.Header().Add("Cache-Status", cacheStatus.String())
		return next.ServeHTTP(w, r)
	}

	if len(meta.Vary) > 0 {
		cacheStatus.Key = cache.GenerateKey(r, h.Key, meta.Vary)
	}

	var entry *cache.Entry
	entry, found = h.getEntry(r.Context(), cacheStatus.Key)
	if !found {
		if len(meta.Vary) > 0 {
			// Found metadata for the original request, but not an entry that matches the regenerated key with the Vary
			// headers, therefore FwdVaryMiss
			cacheStatus.FwdVaryMiss = true
		}
		return h.forward(w, r, cacheStatus, requestTime, next)
	}

	if entry.NeedsRevalidation {
		return h.revalidate(w, r, entry, cacheStatus, requestTime, next)
	}

	if entry.Expires.Before(requestTime) {
		maxStale := time.Duration(h.Timing.MaxStale)
		if respCC.StaleWhileRevalidate != nil {
			maxStale = *respCC.StaleWhileRevalidate
		}
		if !h.Refresh.Disable && entry.Expires.Add(maxStale).After(requestTime) {
			h.backgroundRefresh(r, entry, cacheStatus, requestTime, next)
		} else {
			cacheStatus.FwdStale = true
			return h.forward(w, r, cacheStatus, requestTime, next)
		}
	}

	if reqCC.MaxAge != nil && entry.Date.Add(*reqCC.MaxAge).Before(requestTime) {
		cacheStatus.FwdStale = true
		return h.forward(w, r, cacheStatus, requestTime, next)
	}

	{
		ifNoneMatch := headers.IfNoneMatch{}
		ifNoneMatch.FromHeaders(r.Header["If-None-Match"])

		if !ifNoneMatch.Empty() {
			if ifNoneMatch.Contains(entry.ETag) {
				return h.notModified(w, cacheStatus, requestTime, entry)
			}
			return h.revalidate(w, r, entry, cacheStatus, requestTime, next)
		}
	}

	if respCC.NoCache {
		return h.revalidate(w, r, entry, cacheStatus, requestTime, next)
	}

	cacheStatus.Hit = true
	return h.replyWithEntry(w, cacheStatus, requestTime, entry)
}

// TODO: Handle Authorization Header for RFC 9111
func (h *Handler) shouldBuffer(status int, headers http.Header) bool {
	cacheable := h.handleOriginCacheControl(headers, &cache.Metadata{}, status)
	if cacheable {
		return true
	}

	return status >= 200 && status < 300
}

func (h *Handler) notModified(w http.ResponseWriter, cacheStatus *headers.CacheStatus, requestTime time.Time, e *cache.Entry) error {
	h.entryHeaders(w, cacheStatus, requestTime, e)
	w.WriteHeader(http.StatusNotModified)
	return nil
}

func (h *Handler) entryHeaders(w http.ResponseWriter, cacheStatus *headers.CacheStatus, requestTime time.Time, e *cache.Entry) {
	hs := w.Header()
	for i := range e.Headers {
		hs.Add(e.Headers[i][0], e.Headers[i][1])
	}

	cacheStatus.TTL = e.Expires.Sub(h.now()) + time.Duration(rand.IntN(int(h.Timing.TTLSplay)))
	hs.Add("Cache-Status", cacheStatus.String())

	expires := headers.Expires(requestTime.Add(cacheStatus.TTL))
	hs.Set("Expires", expires.String())

	age := headers.Age(requestTime.Sub(e.Date))
	hs.Set("Age", age.String())

	if e.ETag != "" {
		hs.Set("ETag", e.ETag)
	}
}

func (h *Handler) replyWithEntry(w http.ResponseWriter, cacheStatus *headers.CacheStatus, requestTime time.Time, e *cache.Entry) error {
	h.entryHeaders(w, cacheStatus, requestTime, e)
	w.WriteHeader(e.Status)
	rc := http.NewResponseController(w)
	defer rc.Flush()
	_, err := w.Write(e.Body)
	return err
}
