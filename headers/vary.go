package headers

import (
	"slices"
	"strings"
)

type Vary struct {
	Sorted
}

func (v *Vary) ValsWithout(in []string) (out []string) {
	for i := range in {
		in[i] = strings.ToLower(in[i])
	}

	for i := range v.vals {
		if !slices.Contains(in, v.vals[i]) {
			out = append(out, v.vals[i])
		}
	}

	return
}
