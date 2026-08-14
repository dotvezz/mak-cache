package headers

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CacheControl struct {
	MaxAge               *time.Duration
	MinFresh             *time.Duration
	MaxStale             *time.Duration
	SMaxAge              *time.Duration
	StaleWhileRevalidate *time.Duration
	MustRevalidate       bool
	ProxyRevalidate      bool
	Private              bool
	Public               bool
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

func secsToDuration(s string) (time.Duration, error) {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(i) * time.Second, nil
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
			dur, err := secsToDuration(v)
			cc.MaxAge = &dur
			if err != nil {
				return fmt.Errorf("invalid max-age: %s, %v", d, err)
			}
		case "min-fresh":
			if !ok {
				return fmt.Errorf("invalid max-age: %s", d)
			}
			dur, err := secsToDuration(v)
			cc.MinFresh = &dur
			if err != nil {
				return fmt.Errorf("invalid min-fresh: %s, %v", d, err)
			}
		case "max-stale":
			if !ok {
				return fmt.Errorf("invalid max-age: %s", d)
			}
			dur, err := secsToDuration(v)
			cc.MaxStale = &dur
			if err != nil {
				return fmt.Errorf("invalid max-stale: %s, %v", d, err)
			}
		case "s-maxage":
			if !ok {
				return fmt.Errorf("invalid s-maxage: %s", d)
			}
			dur, err := secsToDuration(v)
			cc.SMaxAge = &dur
			if err != nil {
				return fmt.Errorf("invalid s-maxage: %s, %v", d, err)
			}
		case "stale-while-revalidate":
			if !ok {
				return fmt.Errorf("invalid stale-while-revalidate: %s", d)
			}
			dur, err := secsToDuration(v)
			cc.StaleWhileRevalidate = &dur
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
			cc.Public = true
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
	}

	// private overrides public
	if cc.Public && !cc.Private {
		ds = append(ds, "public")
	}

	if cc.MaxAge != nil {
		ds = append(ds, "max-age="+strconv.Itoa(int(cc.MaxAge.Seconds())))
	}

	if cc.MaxStale != nil {
		ds = append(ds, "max-stale="+strconv.Itoa(int(cc.MaxStale.Seconds())))
	}

	if cc.MinFresh != nil {
		ds = append(ds, "min-fresh="+strconv.Itoa(int(cc.MinFresh.Seconds())))
	}

	if cc.SMaxAge != nil {
		ds = append(ds, "s-maxage="+strconv.Itoa(int(cc.SMaxAge.Seconds())))
	}

	if cc.StaleWhileRevalidate != nil {
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

func (cc *CacheControl) Cacheable(isResponse bool) bool {
	if isResponse {
		return !(cc.NoStore || cc.Private)
	}

	return !(cc.NoStore || cc.NoCache)
}
