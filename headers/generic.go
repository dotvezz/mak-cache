package headers

import (
	"slices"
	"strings"
)

type Sorted struct {
	Generic
}

func (a *Sorted) FromHeaders(v []string) {
	a.Generic.FromHeaders(v)
	slices.Sort(a.vals)
}

type Generic struct {
	vals []string
}

func (a *Generic) String() string {
	return strings.Join(a.vals, ", ")
}

func (a *Generic) FromHeaders(v []string) {
	a.vals = v
	for i := range a.vals {
		a.vals[i] = strings.ToLower(a.vals[i])
	}
}
