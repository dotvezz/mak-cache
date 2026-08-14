package cache

import (
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dotvezz/caddy-cache/cache"
	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/minitime"
	"github.com/dotvezz/caddy-cache/storage"
	"github.com/dotvezz/caddy-cache/storage/otter"
	"github.com/dotvezz/caddy-cache/storage/valkey"
	"github.com/dustin/go-humanize"
	"golang.org/x/sync/singleflight"
)

var (
	_ caddy.Provisioner           = (*Handler)(nil)
	_ caddyhttp.MiddlewareHandler = (*Handler)(nil)
	_ caddyfile.Unmarshaler       = (*Handler)(nil)
	//_ caddy.Validator             = (*Handler)(nil)
)

type caddyfileHelper interface {
	NextBlock(int) bool
	Nesting() int
	Next() bool
	Args(...*string) bool
	Val() string
	NextArg() bool
	ArgErr() error
	RemainingArgs() []string
	Errf(format string, args ...any) error
}

const (
	defaultKey = " default " // Spaces to make it hard to parse an accidentally colliding key from Caddyfile
	moduleName = "cache"
)

func init() {
	caddy.RegisterModule(Handler{})

	httpcaddyfile.RegisterHandlerDirective(moduleName, parseCaddyfile)
	httpcaddyfile.RegisterGlobalOption(moduleName, registerGlobalOption)
	httpcaddyfile.RegisterDirectiveOrder(moduleName, httpcaddyfile.Before, "rewrite")
}

func (h Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.cache",
		New: func() caddy.Module { return new(Handler) },
	}
}

func addStorage[T storage.Storable](p *storage.Provider[T], store storage.Provider[T]) {
	if *p == nil {
		*p = store
	} else {
		*p = storage.Wrap(*p, store)
	}
}

func (h *Handler) Provision(context caddy.Context) (err error) {
	if p, ok := storage.SharedStorageProviders[h.ConfigKey]; ok && p != nil {
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
				return err
			}
			addStorage(&h.entryStorage, store)
		}

		if _, ok = storage.SharedStorageProviders[h.ConfigKey]; ok {
			storage.SharedStorageProviders[h.ConfigKey] = h.entryStorage
		}
	}

	if p, ok := storage.SharedMetadataProviders[h.ConfigKey]; ok && p != nil {
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
				return err
			}
			addStorage(&h.metadataStorage, store)
		}

		if _, ok = storage.SharedMetadataProviders[h.ConfigKey]; ok {
			storage.SharedMetadataProviders[h.ConfigKey] = h.metadataStorage
		}
	}

	h.singleflight = new(singleflight.Group)
	h.now = time.Now
	h.Logger = context.Slogger()

	return nil
}

func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	return parseFromCustomHelper(d, &h.Config)
}

//func (h *Handler) Validate() error {
//	//TODO implement me
//	panic("implement me")
//}

// registerGlobalOption registers global options for the oauth_proxy directive.
func registerGlobalOption(d *caddyfile.Dispenser, existing any) (any, error) {
	d.Next() // Consume the directive name

	if existing == nil { // If configMap is nil, initialize it with a map
		existing = make(map[string]config.Config)
	}

	var (
		configMap map[string]config.Config
		ok        bool
	)
	if configMap, ok = existing.(map[string]config.Config); !ok {
		return nil, d.Errf("invalid configMap type")
	}

	var key string
	if !d.Args(&key) {
		key = defaultKey
	}

	c := config.Config{}
	err := parseFromCustomHelper(d, &c)
	configMap[key] = c
	return configMap, err
}

// parseCaddyfile parses the oauth_proxy directive from Caddyfile.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	hnd := new(Handler)
	h.Next() // consume directive name

	if !h.Args(&hnd.ConfigKey) {
		hnd.ConfigKey = defaultKey
	}

	existing := h.Option(moduleName)
	if existing != nil {
		if _, ok := existing.(map[string]config.Config); !ok {
			return nil, h.Errf("invalid global config type %T, expected %T", existing, map[string]config.Config{})
		}

		if global, ok := existing.(map[string]config.Config)[hnd.ConfigKey]; ok {
			hnd.Config = global

			// If we were able to load a global config, then we also want to share the storage providers across
			// directives which reference it, so we'll create map keys for the shared entry providers to use in
			// Provision
			storage.SharedStorageProviders[hnd.ConfigKey] = nil
			storage.SharedMetadataProviders[hnd.ConfigKey] = nil
		}
	}

	err := parseFromCustomHelper(h, &hnd.Config)
	return hnd, err
}

func parseFromCustomHelper(h caddyfileHelper, c *config.Config) (err error) {
	for h.NextBlock(0) {
		key := h.Val()
		switch key {
		case "headers":
			nesting := h.Nesting()
			for h.NextBlock(nesting) {
				subKey := h.Val()
				switch subKey {
				case "ignore_vary":
					c.Headers.IgnoreVary = h.RemainingArgs()
				case "override_origin_cache_control":
					c.Headers.OverrideOriginCacheControl, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				case "override_client_cache_control":
					c.Headers.OverrideClientCacheControl, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				default:
					return h.Errf("unknown key configuration subkey %q", subKey)
				}
			}
		case "timing":
			if err := parseTimingConfig(h, &c.Timing); err != nil {
				return err
			}
		case "status_timings", "status_timing":
			args := h.RemainingArgs()
			if len(args) == 0 {
				return h.Errf("expected at least one HTTP status code")
			}
			var statusCodes []int
			for _, arg := range args {
				code, err := strconv.Atoi(arg)
				if err != nil {
					return h.Errf("invalid HTTP status code %q: %v", arg, err)
				}
				statusCodes = append(statusCodes, code)
			}
			var t config.TimingConfig
			if err = parseTimingConfig(h, &t); err != nil {
				return err
			}
			if c.StatusTimings == nil {
				c.StatusTimings = make(map[int]config.TimingConfig)
			}
			for _, code := range statusCodes {
				c.StatusTimings[code] = t
			}
		case "etag":
			nesting := h.Nesting()
			for h.NextBlock(nesting) {
				subKey := h.Val()
				switch subKey {
				case "disable":
					c.ETag.Disable, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				case "crc32":
					c.ETag.CRC32, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				case "sha256":
					c.ETag.SHA256, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				default:
					return h.Errf("unknown etag configuration subkey %q", subKey)
				}
			}
		case "key":
			nesting := h.Nesting()
			for h.NextBlock(nesting) {
				subKey := h.Val()
				switch subKey {
				case "components":
					c.Key.Components = h.RemainingArgs()
				case "strip_query_params":
					c.Key.StripQueryParams = h.RemainingArgs()
				case "no_query_sort":
					c.Key.NoQuerySort, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				default:
					return h.Errf("unknown key configuration subkey %q", subKey)
				}
			}
		case "coalesce":
			nesting := h.Nesting()
			for h.NextBlock(nesting) {
				subKey := h.Val()
				switch subKey {
				case "disable":
					c.Coalesce.Disable, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				default:
					return h.Errf("unknown coalesce configuration subkey %q", subKey)
				}
			}
		case "storage":
			s, err := parseStorageConfig(h)
			if err != nil {
				return err
			}
			c.Storage = append(c.Storage, s)
		case "metadata_storage", "metadata":
			s, err := parseStorageConfig(h)
			if err != nil {
				return err
			}
			c.MetadataStorage = append(c.MetadataStorage, s)
		case "refresh":
			nesting := h.Nesting()
			for h.NextBlock(nesting) {
				subKey := h.Val()
				switch subKey {
				case "disable":
					c.Refresh.Disable, err = parseBoolArg(h)
					if err != nil {
						return err
					}
				case "timeout":
					c.Refresh.Timeout, err = parseDurationArg(h)
					if err != nil {
						return err
					}
				default:
					return h.Errf("unknown refresh configuration subkey %q", subKey)
				}
			}
		case "prometheus":
			args := h.RemainingArgs()
			if len(args) > 1 {
				return h.Errf("invalid prometheus arguments: %v", args)
			}
			if len(args) == 1 {
				c.Prometheus.Prefix = args[0]
			}
			nesting := h.Nesting()
			for h.NextBlock(nesting) {
				subKey := h.Val()
				switch subKey {
				case "prefix":
					var prefix string
					if !h.Args(&prefix) {
						return h.ArgErr()
					}
					c.Prometheus.Prefix = prefix
				default:
					return h.Errf("unknown prometheus configuration subkey %q", subKey)
				}
			}
		default:
			return h.Errf("unknown configuration option %q", key)
		}
	}

	return nil
}

// parseBoolArg parses a boolean argument from the caddyfile.
// If there are no args, it assumes the intended value is `true`.
func parseBoolArg(h caddyfileHelper) (bool, error) {
	args := h.RemainingArgs()
	if len(args) == 0 {
		return true, nil
	}
	if len(args) > 1 {
		return false, h.Errf("invalid boolean value: %v", args)
	}
	switch args[0] {
	case "true", "yes", "on":
		return true, nil
	case "false", "no", "off":
		return false, nil
	default:
		return false, h.Errf("invalid boolean value: %q", args[0])
	}
}

func parseDurationArg(h caddyfileHelper) (minitime.Duration, error) {
	args := h.RemainingArgs()
	if len(args) != 1 {
		return 0, h.Errf("expected exactly one duration value, got %v", args)
	}
	d, err := time.ParseDuration(args[0])
	if err != nil {
		return 0, h.Errf("invalid duration value %q: %v", args[0], err)
	}
	return minitime.Duration(d), nil
}

func parseTimingConfig(h caddyfileHelper, t *config.TimingConfig) (err error) {
	nesting := h.Nesting()
	for h.NextBlock(nesting) {
		subKey := h.Val()
		switch subKey {
		case "ttl":
			t.TTL, err = parseDurationArg(h)
			if err != nil {
				return err
			}
		case "max_stale":
			t.MaxStale, err = parseDurationArg(h)
			if err != nil {
				return err
			}
		case "ttl_splay":
			t.TTLSplay, err = parseDurationArg(h)
			if err != nil {
				return err
			}
		default:
			return h.Errf("unknown timing configuration subkey %q", subKey)
		}
	}
	return nil
}

func parseStorageConfig(h caddyfileHelper) (config.StorageConfig, error) {
	var s config.StorageConfig
	args := h.RemainingArgs()
	if len(args) != 1 {
		return s, h.Errf("expected exactly one argument for storage provider name, got %v", args)
	}
	providerName := args[0]
	switch providerName {
	case "otter", "in-memory":
		s.Otter = &config.OtterConfig{}
		nesting := h.Nesting()
		for h.NextBlock(nesting) {
			subKey := h.Val()
			switch subKey {
			case "memory_limit":
				var valStr string
				if !h.Args(&valStr) {
					return s, h.ArgErr()
				}
				val, err := strconv.Atoi(valStr)
				if err != nil {
					var val2 uint64
					val2, err = humanize.ParseBytes(valStr)
					val = int(val2)
				}
				if err != nil {
					return s, h.Errf("invalid memory_limit value %q: %v", valStr, err)
				}
				s.Otter.MemoryLimit = uint64(val)
			default:
				return s, h.Errf("unknown in_memory/otter storage configuration subkey %q", subKey)
			}
		}
	case "valkey", "redis":
		s.Valkey = &config.ValkeyConfig{}
		nesting := h.Nesting()
		for h.NextBlock(nesting) {
			subKey := h.Val()
			switch subKey {
			case "address":
				if !h.Args(&s.Valkey.Address) {
					return s, h.ArgErr()
				}
			default:
				return s, h.Errf("unknown valkey/redis storage configuration subkey %q", subKey)
			}
		}
	default:
		return s, h.Errf("unknown storage provider %q", providerName)
	}
	return s, nil
}
