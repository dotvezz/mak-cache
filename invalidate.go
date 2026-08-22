package cache

import (
	"net/http"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/headers"
)

func (m *Middleware) handleAndInvalidate(w http.ResponseWriter, r *http.Request, cacheStatus *headers.CacheStatus, next http.Handler) {
	w.Header().Add("Cache-Status", cacheStatus.String())

	// Record the headers/status, but don't buffer the response.
	rec := caddyhttp.NewResponseRecorder(w, nil, func(int, http.Header) bool { return false })
	next.ServeHTTP(rec, r)

	if rec.Status() >= 200 && rec.Status() < 400 {
		var e *cache.Entry
		var found bool

		e, found = m.getEntry(r.Context(), cacheStatus.Key)
		if found && e != nil {
			e.NeedsRevalidation = true
			_ = m.updateEntry(r.Context(), cacheStatus.Key, e)
		}

		var meta *cache.Metadata
		if meta, found = m.getMetadata(r.Context(), cacheStatus.Key); found && meta != nil {
			for i := range meta.Linked {
				e, found = m.getEntry(r.Context(), meta.Linked[i])
				if found && e != nil {
					e.NeedsRevalidation = true
					_ = m.updateEntry(r.Context(), meta.Linked[i], e)
				}
			}
		}
	}
}
