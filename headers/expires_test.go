package headers

import (
	"net/http"
	"testing"
	"time"
)

func TestExpires_StringAndFromString(t *testing.T) {
	t.Run("String formatting", func(t *testing.T) {
		tm := time.Date(2025, 10, 20, 12, 0, 0, 0, time.UTC)
		e := Expires(tm)
		got := e.String()
		want := tm.Format(http.TimeFormat)
		if got != want {
			t.Errorf("Expires.String() = %q, want %q", got, want)
		}
	})

	t.Run("FromString valid RFC1123 time", func(t *testing.T) {
		timeStr := "Mon, 20 Oct 2025 12:00:00 GMT"
		var e Expires
		err := e.FromString(timeStr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.String() != timeStr {
			t.Errorf("got %q, want %q", e.String(), timeStr)
		}
	})

	t.Run("FromString invalid time string", func(t *testing.T) {
		var e Expires
		err := e.FromString("invalid-time-format")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
