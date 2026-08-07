package cache

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/headers"
	"github.com/dotvezz/caddy-cache/responses"
)

func (h *Handler) fwdUpstream(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) (e *cache.Entry, err error) {
	e = new(cache.Entry)
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	rec := caddyhttp.NewResponseRecorder(w, buf, h.shouldBuffer)

	err = next.ServeHTTP(rec, r)
	if err != nil {
		return nil, err
	}

	if !rec.Buffered() {
		return nil, errNotBuffered
	}

	e.FromResponse(rec)

	return e, err
}

func (h *Handler) backgroundRefresh(req *http.Request, entry *cache.Entry, cacheStatus *headers.CacheStatus, requestTime time.Time, next caddyhttp.Handler) {
	// After it finishes writing downstream, caddy runs a deferred timeout cancel.
	// Since we're running this in the background, that cancel would be a problem so we'll just ignore it here.
	newCtx := context.WithoutCancel(req.Context())
	req = req.WithContext(newCtx)

	go func() {
		// Since we removed the cancel above, we want to ensure we have a timeout
		ctx, cancel := context.WithTimeout(req.Context(), h.Refresh.ResolveTimeout())
		defer cancel()
		req = req.WithContext(ctx)

		noop := responses.NoopWriter{}

		_ = h.revalidate(noop, req, entry, cacheStatus, requestTime, next)
	}()
}

func (h *Handler) handleUpstreamCacheControl(cc []string, m *cache.Metadata) (cacheable bool) {
	if h.Config.Headers.IgnoreOriginCacheControl {
		return true
	}

	cacheControl := headers.CacheControl{}
	// Only load the most-recent (last in the slice) Cache-Control header
	err := cacheControl.FromString(cc[len(cc)-1])
	if err == nil {
		// Only load Cache-Control into the metadata if we were able to successfully parse it
		m.CacheControl = cacheControl.Directives()
	}

	if cacheControl.MaxAge > 0 {
		m.Expires = time.Now().Add(cacheControl.MaxAge)
	}

	return cacheControl.Cacheable()
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, cacheStatus *headers.CacheStatus, requestTime time.Time, next caddyhttp.Handler) error {
	// Even if we're handling a conditional request, if we're forwarding then we want to attempt to cache the full
	// response instead of just passing the possible 304 down.
	r.Header.Del("If-None-Match")
	r.Header.Del("If-Modified-Since")

	// We're holding on to a clone of the request because we may need to reuse it, for example if the origin
	// response has a Vary header.
	// Because we're living in a Caddy handler, and upstream handlers may mutate the request, the original value of r
	// is not safe for reuse.
	rClone := r.Clone(r.Context())

	oneShot := responses.NewOneShot(w)
	var e any
	var err error
	e, err, cacheStatus.Collapsed = h.singleflight.Do(cacheStatus.Key, func() (any, error) {
		return h.fwdUpstream(oneShot, r, next)
	})

	if err != nil {
		_ = oneShot.Fire()
		if !errors.Is(err, errNotBuffered) {
			return err
		}
		return nil
	}

	entry := e.(*cache.Entry)

	m := &cache.Metadata{
		Date:    requestTime,
		Expires: requestTime.Add(time.Duration(h.Timing.TTL)),
	}

	cacheable := true
	vary := headers.Vary{}
	vary.FromHeaders(oneShot.Header().Values("Vary"))

	m.Vary = vary.ValsWithout(h.Headers.IgnoreVary)
	if slices.Contains(m.Vary, "*") {
		cacheable = false
	}

	if cc := oneShot.Header().Values("Cache-Control"); len(cc) > 0 {
		if !h.handleUpstreamCacheControl(cc, m) {
			cacheable = false
		}
	}

	err = h.setMetadata(r.Context(), cacheStatus.Key, m)

	if len(m.Vary) > 0 {
		keyWithVary := cache.GenerateKey(r, h.Key, m.Vary)
		oneShot.Reset()
		e, err, _ = h.singleflight.Do(keyWithVary, func() (any, error) {
			return h.fwdUpstream(oneShot, r, next)
		})

		entry = e.(*cache.Entry)
	}

	entry.Metadata = *m
	if cacheable && err == nil {
		if !h.ETag.Disable {
			if etag := oneShot.Header().Get("ETag"); etag != "" {
				// Get the etag header from upstream
				entry.ETag = etag
			} else {
				if etagHeader := entry.GetHeader("ETag"); len(etagHeader) > 0 {
					entry.ETag = etagHeader[0]
				} else {
					entry.ETag = cache.GenerateEtag(entry, h.ETag)
				}
				oneShot.Header().Set("ETag", entry.ETag)
			}
		}

		keyWithVary := cache.GenerateKey(rClone, h.Key, m.Vary)
		err = h.setEntry(r.Context(), keyWithVary, entry)
		cacheStatus.Stored = err == nil
	}

	oneShot.Header().Set("Cache-Status", cacheStatus.String())

	oneShot.WriteHeader(entry.Status)

	_, err = oneShot.Write(entry.Body)
	if err != nil {
		return err
	}

	err = oneShot.Fire()
	return err
}
