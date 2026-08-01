package cache

import (
	"net/http"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/cache"
)

func (h *Handler) revalidate(w http.ResponseWriter, r *http.Request, entry *cache.Entry, next caddyhttp.Handler) error {
	// TODO: Do revalidation in a way that isn't just fetching

	return h.forward(w, r, next)
}
