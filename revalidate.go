package mak

import (
	"bytes"
	"errors"
	"net/http"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/headers"
)

func (m *Middleware) revalidate(w http.ResponseWriter, r *http.Request, meta *cache.Metadata, entry *cache.Entry, cacheStatus *headers.CacheStatus, requestTime time.Time, next http.Handler) (err error) {
	// We're holding on to a clone of the original request because we may need to reuse it, for example if the origin
	// response has a Vary header.
	// Because we're living in a Caddy handler, and upstream handlers may mutate the request, the original value of r
	// is not safe for reuse.
	rClone := r.Clone(r.Context())

	var notModified any

	notModified, err, cacheStatus.Collapsed = m.singleflight.Do("revalidate."+cacheStatus.Key, func() (any, error) {
		ifNoneMatch := headers.IfNoneMatch{}
		ifNoneMatch.FromHeaders(r.Header["If-None-Match"])

		buf := bytes.NewBuffer(make([]byte, 0, 1024))
		rec := caddyhttp.NewResponseRecorder(w, buf, m.shouldBuffer)

		next.ServeHTTP(rec, r)

		cacheStatus.FwdStatus = rec.Status()

		if !rec.Buffered() {
			if rec.Status() == http.StatusNotModified {
				cacheable := m.processResponseHeaders(r.Header, rClone.Header, meta, rec.Status())
				if !cacheable {
					return false, nil
				}

				meta.Date = requestTime
				err = m.updateMetadata(r.Context(), cache.GenerateKey(rClone, m.Config.Key, nil), meta)

				entry.Date = requestTime
				entry.Expires = requestTime.Add(time.Duration(m.Timing.TTL))
				entry.Metadata = *meta
				err = m.updateEntry(r.Context(), cacheStatus.Key, entry)
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

		meta.Date = requestTime
		cacheable := m.processResponseHeaders(r.Header, rClone.Header, meta, rec.Status())

		entry.Metadata = *meta
		if cacheable && err == nil {
			if !m.ETag.Disable {
				m.setEtag(w, entry)
			}

			keyWithVary := cache.GenerateKey(rClone, m.Key, meta.Vary)
			err = m.updateEntry(r.Context(), keyWithVary, entry)
			cacheStatus.Stored = err == nil
		}

		err = m.setMetadata(r.Context(), cacheStatus.Key, meta)

		return false, nil
	})

	if err != nil {
		return err
	}

	notModifiedb := notModified.(bool)
	if notModifiedb {
		return nil
	}

	return m.replyWithEntry(w, cacheStatus, requestTime, entry)
}
