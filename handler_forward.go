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

func (h *Handler) handleOriginCacheControl(hdr http.Header, m *cache.Metadata, status int) (cacheable bool) {
	if cc := hdr.Values("Cache-Control"); len(cc) > 0 {
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

		if cacheControl.SMaxAge != nil {
			m.Expires = h.now().Add(*cacheControl.SMaxAge)
		} else if cacheControl.MaxAge != nil {
			m.Expires = h.now().Add(*cacheControl.MaxAge)
		}

		return cacheControl.Cacheable(true)
	}

	if exps := hdr.Get("Expires"); exps != "" {
		expires := headers.Expires{}
		err := expires.FromString(exps)
		if err == nil {
			m.Expires = time.Time(expires)
		} else {
			return false
		}
	}

	// From RFC 9110 15.1
	heuristicallyCacheable := []int{200, 203, 204, 206, 300, 301, 308, 404, 405, 410, 414, 501}
	return slices.Contains(heuristicallyCacheable, status)
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, cacheStatus *headers.CacheStatus, requestTime time.Time, next caddyhttp.Handler) error {
	// Even if we're handling a conditional request, if we're forwarding then we want to attempt to cache the full
	// response instead of just passing the possible 304 down.
	r.Header.Del("If-None-Match")
	r.Header.Del("If-Modified-Since")

	// We're holding on to a clone of the request because we may need to reuse it, for example if the origin
	// response has a Vary header and we need to regenerate a key.
	// Because we're living in a Caddy handler, and upstream handlers may mutate the request, the original value of r
	// is not safe for reuse.
	rClone := r.Clone(r.Context())

	var e any
	var err error
	oneShot := responses.NewOneShot(w)

	e, err, cacheStatus.Collapsed = h.singleflight.Do(cacheStatus.Key, func() (any, error) {
		return h.fwdUpstream(oneShot, r, next)
	})
	if err != nil {
		w.Header().Set("Cache-Status", cacheStatus.String())
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
	vary.FromHeaders(w.Header().Values("Vary"))
	m.Vary = vary.ValsWithout(h.Headers.IgnoreVary)
	if slices.Contains(m.Vary, "*") {
		cacheable = false
	}

	if !h.handleOriginCacheControl(w.Header(), m, entry.Status) {
		cacheable = false
	}

	entry.Metadata = *m
	if cacheable {
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
		m.Linked = append(m.Linked, keyWithVary)
		err = h.setEntry(r.Context(), keyWithVary, entry)
		err = h.setMetadata(r.Context(), cacheStatus.Key, m)
		cacheStatus.Stored = err == nil
	}

	oneShot.WriteHeader(entry.Status)

	_, err = oneShot.Write(entry.Body)
	if err != nil {
		w.Header().Set("Cache-Status", cacheStatus.String())
		err = oneShot.Fire()
		return err
	}

	w.Header().Set("Cache-Status", cacheStatus.String())
	err = oneShot.Fire()
	return err
}
