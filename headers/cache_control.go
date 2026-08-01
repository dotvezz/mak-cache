package headers

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CacheControl struct {
	MaxAge               time.Duration
	StaleWhileRevalidate time.Duration
	MustRevalidate       bool
	ProxyRevalidate      bool
	Private              bool
	NoStore              bool
	NoCache              bool
}

func (cc *CacheControl) String() string {
	return strings.Join(cc.Directives(), ", ")
}

func (cc *CacheControl) FromString(v string) error {
	ds := strings.Split(v, ",")
	for i := range ds {
		ds[i] = strings.ToLower(strings.TrimSpace(ds[i]))
	}

	return cc.FromDirectives(ds)
}

// FromDirectives expects a set of directive strings
func (cc *CacheControl) FromDirectives(ds []string) (err error) {
	for _, d := range ds {
		k, v, ok := strings.Cut(d, "=")
		switch k {
		case "max-age":
			if !ok {
				return fmt.Errorf("invalid max-age: %s", d)
			}
			cc.MaxAge, err = time.ParseDuration(v + "s")
			if err != nil {
				return fmt.Errorf("invalid max-age: %s, %v", d, err)
			}
		case "stale-while-revalidate":
			if !ok {
				return fmt.Errorf("invalid stale-while-revalidate: %s", d)
			}
			cc.StaleWhileRevalidate, err = time.ParseDuration(v + "s")
			if err != nil {
				return fmt.Errorf("invalid stale-while-revalidate: %s, %v", d, err)
			}
		case "must-revalidate":
			cc.MustRevalidate = true
		case "proxy-revalidate":
			cc.ProxyRevalidate = true
		case "no-store":
			cc.NoStore = true
		case "no-cache":
			cc.NoCache = true
		case "private":
			cc.Private = true
		case "public":
			cc.Private = false
		}
	}
	return nil
}

func (cc *CacheControl) Directives() (ds []string) {
	if cc.NoCache {
		ds = append(ds, "no-cache")
	}

	if cc.NoStore {
		ds = append(ds, "no-store")
	}

	if cc.Private {
		ds = append(ds, "private")
	} else {
		ds = append(ds, "public")
	}

	if cc.MaxAge > 0 {
		ds = append(ds, "max-age="+strconv.Itoa(int(cc.MaxAge.Seconds())))
	}

	if cc.StaleWhileRevalidate > 0 {
		ds = append(ds, "stale-while-revalidate="+strconv.Itoa(int(cc.StaleWhileRevalidate.Seconds())))
	}

	if cc.MustRevalidate {
		ds = append(ds, "must-revalidate")
	}

	if cc.ProxyRevalidate {
		ds = append(ds, "proxy-revalidate")
	}

	return ds
}

func (cc *CacheControl) Cacheable() bool {
	return !(cc.NoStore || cc.Private)
}
