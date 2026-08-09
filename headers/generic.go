package headers

import (
	"slices"
	"strings"
)

type CaseSensitive struct {
	Generic
}

func (h *CaseSensitive) FromHeaders(v []string) {
	h.vals = v
}

type Sorted struct {
	Generic
}

func (h *Sorted) FromHeaders(v []string) {
	h.Generic.FromHeaders(v)
	slices.Sort(h.vals)
}

type Generic struct {
	vals []string
}

func (h *Generic) String() string {
	return strings.Join(h.vals, ", ")
}

func (h *Generic) FromHeaders(v []string) {
	h.vals = v
	for i := range h.vals {
		h.vals[i] = strings.ToLower(h.vals[i])
	}
}

func (h *Generic) Contains(v string) bool {
	return slices.Contains(h.vals, v)
}

func (h *Generic) Empty() bool {
	return len(h.vals) == 0
}
