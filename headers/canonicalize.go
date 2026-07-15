package headers

import (
	"net/http"
	"strings"
)

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
