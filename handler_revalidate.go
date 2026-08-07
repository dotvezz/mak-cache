package cache

import (
	"bytes"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/headers"
)

func (h *Handler) revalidate(w http.ResponseWriter, r *http.Request, entry *cache.Entry, cacheStatus *headers.CacheStatus, requestTime time.Time, next caddyhttp.Handler) (err error) {
	// We're holding on to a clone of the original request because we may need to reuse it, for example if the origin
	// response has a Vary header.
	// Because we're living in a Caddy handler, and upstream handlers may mutate the request, the original value of r
	// is not safe for reuse.
	rClone := r.Clone(r.Context())

	var notModified any

	notModified, err, cacheStatus.Collapsed = h.singleflight.Do("revalidate."+cacheStatus.Key, func() (any, error) {
		ifNoneMatch := headers.IfNoneMatch{}
		ifNoneMatch.FromHeaders(r.Header["If-None-Match"])

		e2 := new(cache.Entry)
		e2.Body = []byte{}
		buf := bytes.NewBuffer(e2.Body)
		rec := caddyhttp.NewResponseRecorder(w, buf, h.shouldBuffer)

		err = next.ServeHTTP(rec, r)
		if err != nil {
			return false, err
		}

		if !rec.Buffered() {
			if rec.Status() == http.StatusNotModified {
				entry.Date = requestTime
				entry.Expires = requestTime.Add(time.Duration(h.Timing.TTL))
				return true, nil
			}

			return false, errNotBuffered
		}

		e2.FromResponse(rec)

		if err != nil {
			if errors.Is(err, errNotBuffered) {
				return false, err
			}
			return false, nil
		}

		m := &cache.Metadata{
			Date:    requestTime,
			Expires: requestTime.Add(time.Duration(h.Timing.TTL)),
		}

		cacheable := true
		vary := headers.Vary{}
		vary.FromHeaders(rec.Header().Values("Vary"))

		m.Vary = vary.ValsWithout(h.Headers.IgnoreVary)
		if slices.Contains(m.Vary, "*") {
			cacheable = false
		}

		if cc := rec.Header().Values("Cache-Control"); len(cc) > 0 {
			if !h.handleUpstreamCacheControl(cc, m) {
				cacheable = false
			}
		}

		err = h.setMetadata(r.Context(), cacheStatus.Key, m)

		entry.Metadata = *m
		if cacheable && err == nil {
			if !h.ETag.Disable {
				if etag := rec.Header().Get("ETag"); etag != "" {
					entry.ETag = etag
				} else {
					if etagHeader := entry.GetHeader("ETag"); len(etagHeader) > 0 {
						entry.ETag = etagHeader[0]
					} else {
						entry.ETag = cache.GenerateEtag(entry, h.ETag)
					}
					rec.Header().Set("ETag", entry.ETag)
				}
			}

			keyWithVary := cache.GenerateKey(rClone, h.Key, m.Vary)
			err = h.setEntry(r.Context(), keyWithVary, entry)
			cacheStatus.Stored = err == nil
		}
		return false, nil
	})

	if err != nil {
		return err
	}

	notModifiedb := notModified.(bool)
	if notModifiedb {
		return nil
	}

	return h.replyWithEntry(w, cacheStatus, requestTime, entry)
}
