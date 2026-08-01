package headers

import (
	"math"
	"strconv"
	"strings"
	"time"
)

type CacheStatus struct {
	Hit bool

	// Fwd reasons
	FwdURIMiss  bool
	FwdStale    bool
	FwdVaryMiss bool
	FwdRequest  bool
	FwdBypass   bool
	FwdMethod   bool

	FwdStatus int
	Stored    bool
	Collapsed bool
	Key       string
	Detail    string
	TTL       time.Duration
}

func (cs *CacheStatus) String() string {
	parts := []string{"caddy-cache"}

	if cs.Collapsed {
		parts = append(parts, "collapsed")
	}
	if cs.Hit {
		sec := int(math.Round(cs.TTL.Seconds()))
		parts = append(parts, "hit", "ttl="+strconv.Itoa(sec), "key="+cs.Key)
	} else {
		var reason string
		switch {
		case cs.FwdVaryMiss:
			reason = "vary-miss"
		case cs.FwdURIMiss:
			reason = "uri-miss"
		case cs.FwdRequest:
			reason = "request"
		case cs.FwdBypass:
			reason = "bypass"
		case cs.FwdMethod:
			reason = "method"
		case cs.FwdStale:
			reason = "stale"
		default:
			reason = "miss"
		}
		parts = append(parts, "fwd="+reason)

		if cs.FwdStatus != 0 {
			parts = append(parts, "fwd-status="+reason)
		}
	}
	if cs.Stored {
		parts = append(parts, "stored", "key="+cs.Key)
	}
	if cs.Detail != "" {
		parts = append(parts, "detail="+cs.Detail)
	}

	str := strings.Join(parts, "; ")

	return str
}
