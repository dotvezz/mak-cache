package headers

import (
	"slices"
	"testing"
)

func TestVary_ValsWithout(t *testing.T) {
	v := Vary{}
	v.FromHeaders([]string{"Accept-Encoding", "Accept-Language", "User-Agent"})

	got := v.ValsWithout([]string{"accept-encoding", "user-agent"})
	want := []string{"accept-language"}

	if !slices.Equal(got, want) {
		t.Errorf("Vary.ValsWithout() = %v, want %v", got, want)
	}
}
