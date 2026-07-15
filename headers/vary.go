package headers

import "slices"

type Vary struct {
	Sorted
}

func (v *Vary) ValsWithout(in []string) (out []string) {
	for i := range v.vals {
		if !slices.Contains(in, v.vals[i]) {
			out = append(out, v.vals[i])
		}
	}

	return
}
