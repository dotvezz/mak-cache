package cache

import (
	"bytes"
	"errors"
	"net/http"
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

		buf := bytes.NewBuffer(make([]byte, 0, 1024))
		rec := caddyhttp.NewResponseRecorder(w, buf, h.shouldBuffer)

		err = next.ServeHTTP(rec, r)
		if err != nil {
			return false, err
		}

		cacheStatus.FwdStatus = rec.Status()

		if !rec.Buffered() {
			if rec.Status() == http.StatusNotModified {
				entry.Date = requestTime
				entry.Expires = requestTime.Add(time.Duration(h.Timing.TTL))
				err = h.updateEntry(r.Context(), cacheStatus.Key, entry)
				if err != nil {
					return false, err
				}

				err = h.setMetadata(r.Context(), cacheStatus.Key, &entry.Metadata)
				return true, err
			}

			return false, errNotBuffered
		}

		entry.FromResponse(rec)

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

		cacheable := h.processResponseHeaders(r.Header, rClone.Header, m, rec.Status())

		entry.Metadata = *m
		if cacheable && err == nil {
			if !h.ETag.Disable {
				h.setEtag(w, entry)
			}

			keyWithVary := cache.GenerateKey(rClone, h.Key, m.Vary)
			err = h.updateEntry(r.Context(), keyWithVary, entry)
			cacheStatus.Stored = err == nil
		}

		err = h.setMetadata(r.Context(), cacheStatus.Key, m)

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
