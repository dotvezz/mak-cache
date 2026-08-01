package headers

import (
	"net/http"
	"strings"
)

// CanonicalizeRequest mutates request headers to avoid semantically equivalent but literally different values which can
// increase cache key cardinality.
// This improves cache performance by reducing the memory footprint, as well as improving cache hit ratio.
func CanonicalizeRequest(hs http.Header) {
	for k := range hs {
		switch strings.ToLower(k) {
		case "accept", "accept-language", "cache_control":
			h := Sorted{}
			h.FromHeaders(hs[k])
			hs.Set(k, h.String())
		case "accept-encoding":
			h := AcceptEncoding{}
			h.FromHeaders(hs[k])
			hs.Set(k, h.String())
		}
	}
}
