package cache

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/headers"
)

func GenerateKey(r *http.Request, cfg config.CacheKeyConfig, vary []string) string {
	if r == nil {
		return ""
	}
	cs := cfg.ResolveComponents()
	sb := strings.Builder{}

	usedHeaders := make(map[string]struct{})

	for i := range cs {
		switch cs[i] {
		case "scheme":
			sb.WriteString(r.URL.Scheme)
		case "host":
			host, _, _ := strings.Cut(r.URL.Host, ":")
			sb.WriteString(strings.ToLower(host))
		case "port":
			sb.WriteString(strings.ToLower(afterDelim(r.URL.Host, ':')))
		case "method":
			sb.WriteString(r.Method)
		case "path":
			sb.WriteString(r.URL.Path)
		case "query":
			sb.WriteString(buildQueryString(r, cfg))
		default:
			c := strings.SplitN(cs[i], ".", 2)
			if len(c) < 2 {
				continue
			}
			switch c[0] {
			case "header":
				hk := http.CanonicalHeaderKey(c[1])
				h := headers.Generic{}
				h.FromHeaders(r.Header[hk])
				sb.WriteString(hk)
				sb.WriteString("=")
				sb.WriteString(h.String())
				usedHeaders[hk] = struct{}{}
			}
		}
	}

	if len(vary) > 0 {
		for i := range vary {
			hk := http.CanonicalHeaderKey(vary[i])
			if _, ok := usedHeaders[hk]; !ok {
				h := headers.Generic{}
				h.FromHeaders(r.Header[hk])
				sb.WriteString(hk)
				sb.WriteString("=")
				sb.WriteString(h.String())
			}
		}
	}

	// TODO: configurable hash functions
	hash := md5.Sum([]byte(sb.String()))
	s := hex.EncodeToString(hash[:])
	return s
}

func afterDelim(s string, delim byte) string {
	if i := strings.IndexByte(s, delim); i != -1 {
		return s[i+1:]
	}
	return ""
}

func buildQueryString(r *http.Request, cfg config.CacheKeyConfig) string {
	if cfg.NoQuerySort {
		return buildUnsortedQueryString(r, cfg)
	}

	qs := r.URL.Query()
	for i := range cfg.StripQueryParams {
		qs.Del(cfg.StripQueryParams[i])
	}

	return qs.Encode()
}

func buildUnsortedQueryString(r *http.Request, cfg config.CacheKeyConfig) string {
	q := r.URL.RawQuery
	q = strings.TrimLeft(q, "?")
	if q == "" {
		return ""
	}
	qs := strings.Split(q, "&")

	sb := strings.Builder{}

	for i := range qs {
		parts := strings.Split(qs[i], "=")
		if len(parts) < 1 {
			continue
		}
		key, err := url.QueryUnescape(parts[0])
		if err != nil || len(key) == 0 {
			continue
		}

		if slices.Contains(cfg.StripQueryParams, key) {
			continue
		}

		if len(parts) < 2 {
			sb.WriteString(key + "=")
		} else {
			value, err := url.QueryUnescape(parts[1])
			if err != nil {
				sb.WriteString(key + "=")
			} else {
				sb.WriteString(key + "=" + value)
			}
		}
	}

	return sb.String()
}
