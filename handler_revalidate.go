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
	_, err, cacheStatus.Collapsed = h.singleflight.Do("revalidate."+cacheStatus.Key, func() (any, error) {
		ifNoneMatch := headers.IfNoneMatch{}
		ifNoneMatch.FromHeaders(r.Header["If-None-Match"])

		e2 := new(cache.Entry)
		buf := bytes.NewBuffer(e2.Body)
		rec := caddyhttp.NewResponseRecorder(w, buf, h.shouldBuffer)

		err = next.ServeHTTP(rec, r)
		if err != nil {
			return nil, err
		}

		if !rec.Buffered() {
			if rec.Status() == http.StatusNotModified {
				entry.Expires = requestTime.Add(time.Duration(h.Timing.TTL))
				return nil, nil
			} else {
				return nil, errNotBuffered
			}
		}

		e2.FromResponse(rec)

		if err != nil {
			if errors.Is(err, errNotBuffered) {
				return nil, err
			}
			return nil, nil
		}

		return nil, nil
	})

	if err != nil {
		return err
	}

	return h.replyWithEntry(w, cacheStatus, requestTime, entry)
}
