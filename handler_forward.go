package cache

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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

func (h *Handler) backgroundRefresh(req *http.Request, meta *cache.Metadata, entry *cache.Entry, cacheStatus *headers.CacheStatus, requestTime time.Time, next caddyhttp.Handler) {
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

		_ = h.revalidate(noop, req, meta, entry, cacheStatus, requestTime, next)
	}()
}

func (h *Handler) processResponseHeaders(reqH, respH http.Header, m *cache.Metadata, status int) (cacheable bool) {
	// Handle the Cache-Control header, set metadata and cacheability
	var respCC headers.CacheControl
	if cc := respH.Values("Cache-Control"); len(cc) > 0 {
		if h.Config.Headers.OverrideOriginCacheControl {
			return true
		}

		respCC = headers.CacheControl{}
		// Only load the most-recent (last in the slice) Cache-Control header
		err := respCC.FromString(cc[len(cc)-1])
		if err == nil {
			// Only load Cache-Control into the metadata if we were able to successfully parse it
			m.CacheControl = respCC.Directives()
		}

		if respCC.SMaxAge != nil {
			m.Expires = h.now().Add(*respCC.SMaxAge)
		} else if respCC.MaxAge != nil {
			m.Expires = h.now().Add(*respCC.MaxAge)
		}

		return respCC.Cacheable(true)
	}

	// Only allow caching authorized requests if explicitly allowed in response Cache-Control
	// RFC 9211 Sec 3.5
	if reqH.Get("Authorization") != "" && !respCC.MustRevalidate && !respCC.Public && respCC.SMaxAge == nil {
		return false
	}

	// Handle Expires header, set metadata, and set not cacheable if invalid
	if exps := respH.Get("Expires"); exps != "" {
		expires := headers.Expires{}
		err := expires.FromString(exps)
		if err == nil {
			m.Expires = time.Time(expires)
		} else {
			return false
		}
	}

	// Set Eviction timeline
	m.Evict = m.Expires
	if respCC.MaxStale != nil {
		m.Evict = m.Evict.Add(*respCC.MaxStale)
	} else {
		m.Evict = m.Evict.Add(time.Duration(h.Config.Timing.MaxStale))
	}

	// Not cacheable if Vary contains "*"
	vary := headers.Vary{}
	vary.FromHeaders(respH.Values("Vary"))
	m.Vary = vary.ValsWithout(h.Headers.IgnoreVary)
	if slices.Contains(m.Vary, "*") {
		return false
	}

	// From RFC 9110 15.1
	heuristicallyCacheable := []int{200, 203, 204, 206, 300, 301, 308, 404, 405, 410, 414, 501}
	return slices.Contains(heuristicallyCacheable, status)
}

func (h *Handler) setEtag(resp http.ResponseWriter, entry *cache.Entry) {
	if etag := resp.Header().Get("ETag"); etag != "" {
		// Get the etag header from upstream
		entry.ETag = etag
	} else {
		if etagHeader := entry.GetHeader("ETag"); len(etagHeader) > 0 {
			entry.ETag = etagHeader[0]
		} else {
			entry.ETag = cache.GenerateEtag(entry, h.ETag)
		}
		resp.Header().Set("ETag", entry.ETag)
	}
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

	oneShot := responses.NewOneShot(w)
	var e any
	var err error

	e, err, cacheStatus.Collapsed = h.singleflight.Do(cacheStatus.Key, func() (any, error) {
		return h.fwdUpstream(oneShot, r, next)
	})

	if err != nil {
		oneShot.Header().Set("Cache-Status", cacheStatus.String())
		_ = oneShot.Fire()
		if !errors.Is(err, errNotBuffered) {
			cacheStatus.FwdStatus = oneShot.Status()
			return err
		}
		return nil
	}

	entry := e.(*cache.Entry)

	m := &cache.Metadata{
		Date:    requestTime,
		Expires: requestTime.Add(time.Duration(h.Timing.TTL)),
	}

	cacheable := h.processResponseHeaders(rClone.Header, oneShot.Header(), m, entry.Status)

	entry.Metadata = *m
	if cacheable {
		if !h.ETag.Disable {
			h.setEtag(oneShot, entry)
		}

		keyWithVary := cache.GenerateKey(rClone, h.Key, m.Vary)
		m.Linked = append(m.Linked, keyWithVary)
		err = h.setEntry(r.Context(), keyWithVary, entry)
		cacheStatus.Stored = err == nil
	}

	// Set metadata regardless of entry storeability/cacheability
	err = h.setMetadata(r.Context(), cacheStatus.Key, m)

	oneShot.WriteHeader(entry.Status)

	_, err = oneShot.Write(entry.Body)
	cacheStatus.FwdStatus = entry.Status
	w.Header().Set("Cache-Status", cacheStatus.String())
	if err != nil {
		h.Error("forward",
			slog.String("description", "error writing response body"),
			slog.String("err", err.Error()),
		)
	}

	err = oneShot.Fire()
	return err
}
