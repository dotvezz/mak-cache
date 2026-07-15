package headers

import (
	"strconv"
	"strings"
	"time"
)

type CacheControl struct {
	TTL      time.Duration
	StaleTTL time.Duration
	Private  bool
	NoStore  bool
	NoCache  bool
}

func (cc *CacheControl) String() string {
	parts := []string{}

	if cc.NoCache {
		parts = append(parts, "no-cache")
	}

	if cc.NoStore {
		parts = append(parts, "no-store")
	}

	if cc.Private {
		parts = append(parts, "private")
	} else {
		parts = append(parts, "public")
	}

	parts = append(parts, "public", "max-age="+strconv.Itoa(int(cc.TTL.Seconds())))
	if cc.StaleTTL > 0 {
		parts = append(parts, "stale-while-revalidate="+strconv.Itoa(int(cc.TTL.Seconds())))
	}

	return strings.Join(parts, ", ")
}
