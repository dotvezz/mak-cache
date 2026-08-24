package headers

import "testing"

func TestGeneric(t *testing.T) {
	t.Run("FromHeaders lowercases headers", func(t *testing.T) {
		g := Generic{}
		g.FromHeaders([]string{"Accept-Encoding", "User-Agent"})
		if g.String() != "accept-encoding, user-agent" {
			t.Errorf("Generic.String() = %q, want 'accept-encoding, user-agent'", g.String())
		}
	})

	t.Run("Contains method", func(t *testing.T) {
		g := Generic{}
		g.FromHeaders([]string{"gzip", "br"})
		if !g.Contains("gzip") {
			t.Error("expected Contains('gzip') to be true")
		}
		if g.Contains("deflate") {
			t.Error("expected Contains('deflate') to be false")
		}
	})

	t.Run("Empty method", func(t *testing.T) {
		g := Generic{}
		if !g.Empty() {
			t.Error("expected Empty() to be true for uninitialized Generic")
		}
		g.FromHeaders([]string{"gzip"})
		if g.Empty() {
			t.Error("expected Empty() to be false after FromHeaders")
		}
	})
}

func TestCaseSensitive(t *testing.T) {
	cs := CaseSensitive{}
	cs.FromHeaders([]string{"ETag", "W/\"123\""})
	if cs.String() != "ETag, W/\"123\"" {
		t.Errorf("CaseSensitive.String() = %q, want 'ETag, W/\"123\"'", cs.String())
	}
}

func TestSorted(t *testing.T) {
	s := Sorted{}
	s.FromHeaders([]string{"gzip", "br", "deflate"})
	if s.String() != "br, deflate, gzip" {
		t.Errorf("Sorted.String() = %q, want 'br, deflate, gzip'", s.String())
	}
}
