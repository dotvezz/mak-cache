package caddy

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	
	"github.com/dustin/go-humanize"

	mak "github.com/dotvezz/mak-cache"
	"github.com/dotvezz/mak-cache/config"
	"github.com/dotvezz/mak-cache/minitime"
	"github.com/dotvezz/mak-cache/storage"
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

type Handler struct {
	Logger    *slog.Logger
	Config    config.Config
	ConfigKey string

	middleware func(http.Handler) http.Handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) (err error) {
	httpNext := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = next.ServeHTTP(w, r)
	})
	h.middleware(httpNext).ServeHTTP(w, r)
	return
}

func (h Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.cache",
		New: func() caddy.Module { return new(Handler) },
	}
}

func (h *Handler) Provision(_ caddy.Context) (err error) {
	h.middleware, err = mak.New()
	return
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
		case "entry_storage", "entries", "storage":
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
	var valStr string
	var s config.StorageConfig
	args := h.RemainingArgs()
	if len(args) < 1 || len(args) > 2 {
		return s, h.Errf("expected one or two arguments for storage provider name, got %d: %v", len(args), args)
	}
	providerName := args[0]
	switch providerName {
	case "otter", "in_memory":
		s.Otter = &config.OtterConfig{}
		if len(args) == 1 {
			nesting := h.Nesting()
			for h.NextBlock(nesting) {
				subKey := h.Val()
				switch subKey {
				case "memory_limit":
					if !h.Args(&valStr) {
						return s, h.ArgErr()
					}
				default:
					return s, h.Errf("unknown in_memory/otter storage configuration subkey %q", subKey)
				}
			}
		} else {
			valStr = args[1]
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
	case "valkey", "redis":
		s.Valkey = &config.ValkeyConfig{}
		if len(args) == 1 {
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
		} else {
			s.Valkey.Address = args[1]
		}
	default:
		return s, h.Errf("unknown storage provider %q", providerName)
	}
	return s, nil
}
