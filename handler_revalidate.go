package cache

import (
	"net/http"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/headers"
)

func (h *Handler) revalidate(w http.ResponseWriter, r *http.Request, entry *cache.Entry, cacheStatus headers.CacheStatus, requestTime time.Time, next caddyhttp.Handler) error {
	// TODO: Do revalidation in a way that isn't just fetching

	return h.forward(w, r, cacheStatus, requestTime, next)
}
