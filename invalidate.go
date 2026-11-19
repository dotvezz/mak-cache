package mak

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

		er := m.getEntry(r.Context(), cacheStatus.Key)
		e, found = er.entry, er.found
		if found && e != nil {
			e.NeedsRevalidation = true
			_ = m.updateEntry(r.Context(), cacheStatus.Key, e)
		}

		if metaResult := m.getMetadata(r.Context(), cacheStatus.Key); found && metaResult.metadata != nil {
			for i := range metaResult.metadata.Linked {
				er = m.getEntry(r.Context(), metaResult.metadata.Linked[i])
				e, found = er.entry, er.found
				if found && e != nil {
					e.NeedsRevalidation = true
					_ = m.updateEntry(r.Context(), metaResult.metadata.Linked[i], e)
				}
			}
		}
	}
}
