package cache

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/headers"
	"github.com/dotvezz/caddy-cache/responses"
)

func (h *Handler) toUpstream(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) (e *cache.Entry, err error) {
	bs := make([]byte, 0, 1024)
	buf := bytes.NewBuffer(bs)
	rec := caddyhttp.NewResponseRecorder(w, buf, h.shouldBuffer)

	err = next.ServeHTTP(rec, r)
	if err != nil {
		return nil, err
	}

	if !rec.Buffered() {
		return nil, errNotBuffered
	}

	e = new(cache.Entry)
	e.FromResponse(rec)

	return e, err
}

func (h *Handler) backgroundRefresh(r *http.Request, next caddyhttp.Handler) {
	// After it finishes writing downstream, caddy runs a deferred timeout cancel.
	// Since we're running this in the background, that cancel would be a problem so we'll just ignore it here.
	newCtx := context.WithoutCancel(r.Context())
	r = r.WithContext(newCtx)

	go func() {
		// Since we removed the cancel above, we want to ensure we have a timeout
		ctx, cancel := context.WithTimeout(r.Context(), h.Refresh.ResolveTimeout())
		defer cancel()
		r = r.WithContext(ctx)

		noop := responses.NoopWriter{}

		_ = h.forward(noop, r, next)
	}()
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	originalRequest := getOriginalRequest(r.Context())
	key := cache.GenerateKey(originalRequest, h.Key, nil)
	oneShot := responses.NewOneShot(w)
	e, err, collapsed := h.singleflight.Do(key, func() (any, error) {
		return h.toUpstream(oneShot, r, next)
	})

	if err != nil {
		_ = oneShot.Fire()
		if !errors.Is(err, errNotBuffered) {
			return err
		}
		return nil
	}

	entry := e.(*cache.Entry)

	cacheStatus := getCacheStatus(r.Context())
	cacheStatus.Collapsed = collapsed

	m := &cache.Metadata{
		Date:    getRequestTime(r.Context()),
		Expires: getRequestTime(r.Context()).Add(time.Duration(h.Timing.TTL)),
	}

	cacheable := true
	vary := headers.Vary{}
	vary.FromHeaders(oneShot.Header().Values("Vary"))
	m.Vary = vary.ValsWithout(h.IgnoreVaryHeaders)
	if slices.Contains(m.Vary, "*") {
		cacheable = false
	}

	err = h.setMetadata(r.Context(), key, m)

	if len(m.Vary) > 0 {
		keyWithVary := cache.GenerateKey(originalRequest, h.Key, m.Vary)
		oneShot.Reset()
		e, err, _ = h.singleflight.Do(keyWithVary, func() (any, error) {
			return h.toUpstream(oneShot, r, next)
		})

		entry = e.(*cache.Entry)
	}

	entry.Metadata = *m
	if cacheable && err == nil {
		if !h.ETag.Disable {
			if etag := oneShot.Header().Get("ETag"); etag != "" {
				entry.ETag = etag
			} else {
				if etagHeader := entry.GetHeader("ETag"); len(etagHeader) > 0 {
					entry.ETag = etagHeader[0]
				} else {
					entry.ETag = cache.GenerateEtag(entry, h.ETag)
				}
				oneShot.Header().Set("ETag", entry.ETag)
			}
		}

		keyWithVary := cache.GenerateKey(originalRequest, h.Key, m.Vary)
		err = h.entryStorage.Set(r.Context(), keyWithVary, entry)
		cacheStatus.Stored = err == nil
	}

	oneShot.Header().Set("Cache-Status", cacheStatus.String())

	oneShot.WriteHeader(entry.Status)

	_, err = oneShot.Write(entry.Body)
	if err != nil {
		return err
	}

	err = oneShot.Fire()
	return err
}
