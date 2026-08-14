package headers

import (
	"slices"
	"strings"
)

type AcceptEncoding struct {
	Sorted
}

var acceptEncodingRank = []string{
	"dcz",
	"zstd",
	"dcb",
	"br",
	"gzip",
	"deflate",
	"identity",
}

func (ae *AcceptEncoding) String() string {
	if len(ae.vals) == 0 || len(ae.vals) == 1 && len(ae.vals[0]) == 0 {
		// Empty is equivalent to "*", so we normalize to avoid duplicate-equivalent cache entries
		return "*"
	}

	if slices.Contains(ae.vals, "*") {
		// Including "*" is equivalent to only "*", so we normalize to avoid duplicate-equivalent cache entries
		return "*"
	}

	vals := make([]string, 0)
	for i := range acceptEncodingRank {
		if slices.Contains(ae.vals, acceptEncodingRank[i]) {
			vals = append(vals, acceptEncodingRank[i])
		}
	}

	return strings.Join(vals, ", ")
}
