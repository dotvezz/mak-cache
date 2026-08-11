package cache

import (
	"net/http"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/headers"
)

func (h *Handler) serveAndInvalidate(w http.ResponseWriter, r *http.Request, cacheStatus *headers.CacheStatus, next caddyhttp.Handler) error {
	w.Header().Add("Cache-Status", cacheStatus.String())

	// Record the headers/status, but don't buffer the response.
	rec := caddyhttp.NewResponseRecorder(w, nil, func(int, http.Header) bool { return false })
	err := next.ServeHTTP(rec, r)

	if err == nil && rec.Status() >= 200 && rec.Status() < 400 {
		var e *cache.Entry
		var found bool

		e, found = h.getEntry(r.Context(), cacheStatus.Key)
		if found && e != nil {
			e.NeedsRevalidation = true
			_ = h.updateEntry(r.Context(), cacheStatus.Key, e)
		}

		var meta *cache.Metadata
		if meta, found = h.getMetadata(r.Context(), cacheStatus.Key); found && meta != nil {
			for i := range meta.Linked {
				e, found = h.getEntry(r.Context(), meta.Linked[i])
				if found && e != nil {
					e.NeedsRevalidation = true
					_ = h.updateEntry(r.Context(), meta.Linked[i], e)
				}
			}
		}
	}

	return err
}
