package config

import "time"

/*
	As stated in the package comment, the philosophy for configuring mak-cache aligns with two things:
		1. The Go Proverb, "Make the zero value useful"
		2. The Principle of Least Surprise
	That means that we strive to define configuration types in such a way that the zero value should be the most
	sensible default.

	This file exists to be a clear, readable, and well-documented set of _defined exceptions_ to that rule.
*/

var (
	// defaultCacheKeyComponents is the default list of cache key components to include in the cache key.
	defaultCacheKeyComponents = []string{"host", "path", "query"}
	defaultRefreshTimeout     = time.Second * 30
)

// ResolveComponents returns the list of cache key components to include in the cache key.
// This is the only config value where the "zero" value isn't itself designed to be a sane default; if
// CacheKeyConfig.Components is nil, the result of DefaultCacheKeyComponents() is returned.
func (c CacheKeyConfig) ResolveComponents() []string {
	if c.Components == nil {
		return defaultCacheKeyComponents
	}
	return c.Components
}

// ResolveTimeout returns the default background refresh timeout
func (r *RefreshConfig) ResolveTimeout() time.Duration {
	if r.Timeout == 0 {
		return defaultRefreshTimeout
	}

	return time.Duration(r.Timeout)
}
