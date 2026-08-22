package cache

import (
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"slices"
	"time"

	"github.com/dotvezz/mak-cache/requests"
	"github.com/dotvezz/mak-cache/storage/otter"
	"github.com/dotvezz/mak-cache/storage/valkey"
	"golang.org/x/sync/singleflight"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/config"
	"github.com/dotvezz/mak-cache/headers"
	"github.com/dotvezz/mak-cache/storage"
)

var (
	defaultKey    = " default " // Spaces to make it hard to parse an accidentally colliding key from Caddyfile
	defaultConfig = config.Config{
		Storage: []config.StorageConfig{
			{
				Otter: &config.OtterConfig{
					MemoryLimit: 1024 * 1024 * 1024, // 1GB
				},
			},
		},
	}
	errNotBuffered = errors.New("not buffered")
	now            = time.Now
)

type Middleware struct {
	*slog.Logger
	config.Config

	metadataStorage storage.Provider[*cache.Metadata]
	entryStorage    storage.Provider[*cache.Entry]

	singleflight *singleflight.Group
}

type options struct {
	logger    *slog.Logger
	config    config.Config
	configKey string
}

func addStorage[T storage.Storable](p *storage.Provider[T], store storage.Provider[T]) {
	if *p == nil {
		*p = store
	} else {
		*p = storage.Wrap(*p, store)
	}
}

func New(optFuncs ...func(o *options)) (middleware func(next http.Handler) http.Handler, err error) {
	o := options{
		configKey: defaultKey,
		config:    defaultConfig,
		logger:    slog.Default(),
	}
	for _, f := range optFuncs {
		f(&o)
	}

	h := &Middleware{
		Logger: o.logger,
		Config: o.config,

		singleflight: new(singleflight.Group),
	}

	if p, ok := storage.SharedStorageProviders[o.configKey]; ok && p != nil {
		// Try to see if there's a registered shared provider for the current config key. This would be if
		// the config is defined in the global/server block of a Caddy file
		h.entryStorage = p
	} else {
		for _, cfg := range h.Config.Storage {
			var store storage.Provider[*cache.Entry]
			switch {
			case cfg.Otter != nil:
				store, err = otter.NewProvider[*cache.Entry](*cfg.Otter)
			case cfg.Valkey != nil:
				store, err = valkey.NewProvider[cache.Entry, *cache.Entry](*cfg.Valkey)
			}
			if err != nil {
				return nil, err
			}
			addStorage(&h.entryStorage, store)
		}

		if _, ok = storage.SharedStorageProviders[o.configKey]; ok {
			storage.SharedStorageProviders[o.configKey] = h.entryStorage
		}
	}

	if p, ok := storage.SharedMetadataProviders[o.configKey]; ok && p != nil {
		// Try to see if there's a registered shared provider for the current config key. This would be if
		// the config is defined in the global/server block of a Caddy file
		h.metadataStorage = p
	} else {
		for _, cfg := range h.Config.MetadataStorage {
			var store storage.Provider[*cache.Metadata]
			switch {
			case cfg.Otter != nil:
				store, err = otter.NewProvider[*cache.Metadata](*cfg.Otter)
			case cfg.Valkey != nil:
				store, err = valkey.NewProvider[cache.Metadata, *cache.Metadata](*cfg.Valkey)
			}
			if err != nil {
				return nil, err
			}
			addStorage(&h.metadataStorage, store)
		}

		if _, ok = storage.SharedMetadataProviders[o.configKey]; ok {
			storage.SharedMetadataProviders[o.configKey] = h.metadataStorage
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			h.ServeHTTP(writer, request, next)
		})
	}, nil
}

func requestRequiresRevalidation(reqCC headers.CacheControl, entry *cache.Entry, requestTime time.Time) bool {
	ttl := entry.Expires.Sub(requestTime)
	if reqCC.MinFresh != nil && *reqCC.MinFresh > ttl {
		return true
	}

	if reqCC.MaxStale != nil && -ttl > *reqCC.MaxStale {
		return true
	}

	age := requestTime.Sub(entry.Date)
	if reqCC.MaxAge != nil && age > *reqCC.MaxAge {
		return true
	}

	return false
}

func isStale(entry *cache.Entry, requestTime time.Time) bool {
	if entry.Expires.Before(requestTime) {
		return true
	}

	return false
}

func (m *Middleware) canBackgroundRefresh(respCC headers.CacheControl, entry *cache.Entry, requestTime time.Time) bool {
	maxStale := time.Duration(m.Timing.MaxStale)

	if respCC.StaleWhileRevalidate != nil && !m.Headers.OverrideOriginCacheControl {
		maxStale = *respCC.StaleWhileRevalidate
	}

	return !m.Refresh.Disable && entry.Expires.Add(maxStale).After(requestTime)
}

func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.Handler) {
	cacheStatus := new(headers.CacheStatus)
	requestTime := now()

	reqCC := headers.CacheControl{}
	{
		cc := r.Header.Values("Cache-Control")
		if len(cc) > 0 {
			err := reqCC.FromString(cc[len(cc)-1])
			if err != nil {
				m.Info("Request Cache-Control",
					slog.String("error", err.Error()),
					slog.String("Cache-Control", cc[len(cc)-1]),
				)
				// If there was some error parsing Cache-Control, we'll just ignore/strip it and continue
				reqCC = headers.CacheControl{}
				r.Header.Del("Cache-Control")
			}
		}
	}

	if !reqCC.Cacheable(false) {
		cacheStatus.FwdBypass = true
		w.Header().Add("Cache-Status", cacheStatus.String())
		next.ServeHTTP(w, r)
		return
	}

	headers.CanonicalizeRequest(r.Header)
	cacheStatus.Key = cache.GenerateKey(r, m.Key, nil)

	meta, found := m.getMetadata(r.Context(), cacheStatus.Key)

	if !requests.IsSafeMethod(r.Method) {
		cacheStatus.FwdMethod = true
		// Found metadata, but forwarded because of an unsafe method, so we need to invalidate
		// RFC 9211 Sec 4.4
		m.handleAndInvalidate(w, r, cacheStatus, next)
	} else if !found {
		// Miss
		cacheStatus.FwdURIMiss = true
		err := m.forward(w, r, cacheStatus, requestTime, next)
		if err != nil {
			m.Error("ServeHTTP", slog.String("error", err.Error()))
			return
		}
	}

	respCC := headers.CacheControl{}
	err := respCC.FromDirectives(meta.CacheControl)
	if err != nil {
		m.Info("Cache-Control", slog.String("error", err.Error()))
	}

	if slices.Contains(meta.Vary, "*") || !respCC.Cacheable(true) {
		cacheStatus.FwdBypass = true
		w.Header().Add("Cache-Status", cacheStatus.String())
		next.ServeHTTP(w, r)
		return
	}

	if len(meta.Vary) > 0 {
		cacheStatus.Key = cache.GenerateKey(r, m.Key, meta.Vary)
	}

	var entry *cache.Entry
	entry, found = m.getEntry(r.Context(), cacheStatus.Key)
	if !found {
		if len(meta.Vary) > 0 {
			// Found metadata for the original request, but not an entry that matches the regenerated key with the Vary
			// headers, therefore FwdVaryMiss
			cacheStatus.FwdVaryMiss = true
		}
		err = m.forward(w, r, cacheStatus, requestTime, next)
		if err != nil {
			m.Error("ServeHTTP", slog.String("error", err.Error()))
		}
		return
	}

	if requestRequiresRevalidation(reqCC, entry, requestTime) {
		cacheStatus.FwdRequest = true
		err = m.revalidate(w, r, meta, entry, cacheStatus, requestTime, next)
		if err != nil {
			m.Error("ServeHTTP", slog.String("error", err.Error()))
		}
		return
	}

	if entry.NeedsRevalidation {
		err = m.revalidate(w, r, meta, entry, cacheStatus, requestTime, next)
		if err != nil {
			m.Error("ServeHTTP", slog.String("error", err.Error()))
		}
		return
	}

	if isStale(entry, requestTime) {
		if m.canBackgroundRefresh(respCC, entry, requestTime) {
			m.backgroundRefresh(r, meta, entry, cacheStatus, requestTime, next)
		} else {
			cacheStatus.FwdStale = true
			err = m.forward(w, r, cacheStatus, requestTime, next)
			if err != nil {
				m.Error("ServeHTTP", slog.String("error", err.Error()))
			}
			return
		}
	}

	{
		ifNoneMatch := headers.IfNoneMatch{}
		ifNoneMatch.FromHeaders(r.Header["If-None-Match"])

		if !ifNoneMatch.Empty() {
			if ifNoneMatch.Contains(entry.ETag) {
				err = m.notModified(w, cacheStatus, requestTime, entry)
				if err != nil {
					m.Error("ServeHTTP", slog.String("error", err.Error()))
				}
				return
			}
			err = m.revalidate(w, r, meta, entry, cacheStatus, requestTime, next)
			if err != nil {
				m.Error("ServeHTTP", slog.String("error", err.Error()))
			}
			return
		}
	}

	if respCC.NoCache {
		err = m.revalidate(w, r, meta, entry, cacheStatus, requestTime, next)
		if err != nil {
			m.Error("ServeHTTP", slog.String("error", err.Error()))
		}
		return
	}

	cacheStatus.Hit = true
	err = m.replyWithEntry(w, cacheStatus, requestTime, entry)
	if err != nil {
		m.Error("ServeHTTP", slog.String("error", err.Error()))
	}
}

func (m *Middleware) shouldBuffer(status int, headers http.Header) bool {
	cacheable := m.processResponseHeaders(make(http.Header), headers, &cache.Metadata{}, status)
	if cacheable {
		return true
	}

	return status >= 200 && status < 300
}

func (m *Middleware) notModified(w http.ResponseWriter, cacheStatus *headers.CacheStatus, requestTime time.Time, e *cache.Entry) error {
	m.entryHeaders(w, cacheStatus, requestTime, e)
	w.WriteHeader(http.StatusNotModified)
	return nil
}

func (m *Middleware) entryHeaders(w http.ResponseWriter, cacheStatus *headers.CacheStatus, requestTime time.Time, e *cache.Entry) {
	hs := w.Header()
	for i := range e.Headers {
		hs.Add(e.Headers[i][0], e.Headers[i][1])
	}

	cacheStatus.TTL = e.Expires.Sub(now())
	if m.Timing.TTLSplay > 0 {
		cacheStatus.TTL = e.Expires.Sub(now()) + time.Duration(rand.IntN(int(m.Timing.TTLSplay)))
	}

	hs.Add("Cache-Status", cacheStatus.String())

	expires := headers.Expires(requestTime.Add(cacheStatus.TTL))
	hs.Set("Expires", expires.String())

	age := headers.Age(requestTime.Sub(e.Date))
	hs.Set("Age", age.String())

	if e.ETag != "" {
		hs.Set("ETag", e.ETag)
	}
}

func (m *Middleware) replyWithEntry(w http.ResponseWriter, cacheStatus *headers.CacheStatus, requestTime time.Time, e *cache.Entry) error {
	m.entryHeaders(w, cacheStatus, requestTime, e)
	w.WriteHeader(e.Status)
	rc := http.NewResponseController(w)
	defer rc.Flush()
	_, err := w.Write(e.Body)
	return err
}
