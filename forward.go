package mak

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/headers"
	"github.com/dotvezz/mak-cache/responses"
)

func (m *Middleware) fwdUpstream(w http.ResponseWriter, r *http.Request, next http.Handler) (e *cache.Entry, err error) {
	e = new(cache.Entry)

	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	rec := caddyhttp.NewResponseRecorder(w, buf, m.shouldBuffer)

	next.ServeHTTP(rec, r)

	if !rec.Buffered() {
		return nil, errNotBuffered
	}

	e.FromResponse(rec)

	return e, err
}

func (m *Middleware) backgroundRefresh(req *http.Request, meta *cache.Metadata, entry *cache.Entry, cacheStatus *headers.CacheStatus, requestTime time.Time, next http.Handler) {
	// After it finishes writing downstream, caddy runs a deferred timeout cancel.
	// Since we're running this in the background, that cancel would be a problem so we'll just ignore it here.
	newCtx := context.WithoutCancel(req.Context())
	req = req.WithContext(newCtx)

	go func() {
		// Since we removed the cancel above, we want to ensure we have a timeout
		ctx, cancel := context.WithTimeout(req.Context(), m.Refresh.ResolveTimeout())
		defer cancel()
		req = req.WithContext(ctx)

		noop := responses.NoopWriter{}

		_ = m.revalidate(noop, req, meta, entry, cacheStatus, requestTime, next)
	}()
}

func (m *Middleware) processResponseHeaders(reqH, respH http.Header, md *cache.Metadata, status int) (cacheable bool) {
	// Handle the Cache-Control header, set metadata and cacheability
	var respCC headers.CacheControl
	if cc := respH.Values("Cache-Control"); len(cc) > 0 {
		if m.Config.Headers.OverrideOriginCacheControl {
			return true
		}

		respCC = headers.CacheControl{}
		// Only load the most-recent (last in the slice) Cache-Control header
		err := respCC.FromString(cc[len(cc)-1])
		if err == nil {
			// Only load Cache-Control into the metadata if we were able to successfully parse it
			md.CacheControl = respCC.Directives()
		}

		if respCC.SMaxAge != nil {
			md.Expires = now().Add(*respCC.SMaxAge)
		} else if respCC.MaxAge != nil {
			md.Expires = now().Add(*respCC.MaxAge)
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
			md.Expires = time.Time(expires)
		} else {
			return false
		}
	}

	// Set Eviction timeline
	md.Evict = md.Expires
	if respCC.MaxStale != nil {
		md.Evict = md.Evict.Add(*respCC.MaxStale)
	} else {
		md.Evict = md.Evict.Add(time.Duration(m.Config.Timing.MaxStale))
	}

	// Not cacheable if Vary contains "*"
	vary := headers.Vary{}
	vary.FromHeaders(respH.Values("Vary"))
	md.Vary = vary.ValsWithout(m.Headers.IgnoreVary)
	if slices.Contains(md.Vary, "*") {
		return false
	}

	// From RFC 9110 15.1
	heuristicallyCacheable := []int{200, 203, 204, 206, 300, 301, 308, 404, 405, 410, 414, 501}
	return slices.Contains(heuristicallyCacheable, status)
}

func (m *Middleware) setEtag(resp http.ResponseWriter, entry *cache.Entry) {
	if etag := resp.Header().Get("ETag"); etag != "" {
		// Get the etag header from upstream
		entry.ETag = etag
	} else {
		if etagHeader := entry.GetHeader("ETag"); len(etagHeader) > 0 {
			entry.ETag = etagHeader[0]
		} else {
			entry.ETag = cache.GenerateEtag(entry, m.ETag)
		}
		resp.Header().Set("ETag", entry.ETag)
	}
}

func (m *Middleware) forward(w http.ResponseWriter, r *http.Request, cacheStatus *headers.CacheStatus, requestTime time.Time, next http.Handler) error {
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

	e, err, cacheStatus.Collapsed = m.singleflight.Do(cacheStatus.Key, func() (any, error) {
		return m.fwdUpstream(oneShot, r, next)
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

	md := &cache.Metadata{
		Date:    requestTime,
		Expires: requestTime.Add(time.Duration(m.Timing.TTL)),
	}

	cacheable := m.processResponseHeaders(rClone.Header, oneShot.Header(), md, entry.Status)

	entry.Metadata = *md
	if cacheable {
		if !m.ETag.Disable {
			m.setEtag(oneShot, entry)
		}

		keyWithVary := cache.GenerateKey(rClone, m.Key, md.Vary)
		md.Linked = append(md.Linked, keyWithVary)
		err = m.setEntry(r.Context(), keyWithVary, entry)
		cacheStatus.Stored = err == nil
	}

	// Set metadata regardless of entry storeability/cacheability
	err = m.setMetadata(r.Context(), cacheStatus.Key, md)

	oneShot.WriteHeader(entry.Status)

	_, err = oneShot.Write(entry.Body)
	cacheStatus.FwdStatus = entry.Status
	w.Header().Set("Cache-Status", cacheStatus.String())
	if err != nil {
		m.Error("forward",
			slog.String("description", "error writing response body"),
			slog.String("err", err.Error()),
		)
	}

	err = oneShot.Fire()
	return err
}
