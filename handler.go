package cache

import (
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"slices"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

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

	// TODO: configurable caching rules for not-traditionally-cacheable methods
	if r.Method != http.MethodHead && r.Method != http.MethodGet {
		cacheStatus.FwdMethod = true
		w.Header().Add("Cache-Status", cacheStatus.String())
		return next.ServeHTTP(w, r)
	}

	headers.CanonicalizeRequest(r.Header)
	cacheStatus.Key = cache.GenerateKey(r, h.Key, nil)

	meta, found := h.getMetadata(r.Context(), cacheStatus.Key)

	if !found {
		cacheStatus.FwdURIMiss = true
		return h.forward(w, r, cacheStatus, requestTime, next)
	}

	cacheControl := headers.CacheControl{}
	err = cacheControl.FromDirectives(meta.CacheControl)
	if err != nil {
		h.Info("Cache-Control", slog.String("error", err.Error()))
	}

	if slices.Contains(meta.Vary, "*") || !cacheControl.Cacheable() {
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
			// Strictly speaking, this could be a miss due to cache eviction, but _most likely_ to be a Vary miss
			cacheStatus.FwdVaryMiss = true
		}
		return h.forward(w, r, cacheStatus, requestTime, next)
	}

	if entry.Expires.Before(h.now()) {
		if !h.Refresh.Disable && entry.Expires.Before(h.now().Add(cacheControl.StaleWhileRevalidate)) {
			h.backgroundRefresh(r, entry, cacheStatus, requestTime, next)
		} else {
			cacheStatus.FwdStale = true
			return h.forward(w, r, cacheStatus, requestTime, next)
		}
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

	cacheStatus.Hit = true
	return h.replyWithEntry(w, cacheStatus, requestTime, entry)
}

func (h *Handler) shouldBuffer(status int, _ http.Header) bool {
	// TODO: Handle negative caching
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
	hs.Add("Expires", expires.String())

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
