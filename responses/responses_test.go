package responses

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoopWriter(t *testing.T) {
	nw := NoopWriter{}

	if h := nw.Header(); h == nil {
		t.Error("expected non-nil header from NoopWriter")
	}

	n, err := nw.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("Write() = (%d, %v), want (5, nil)", n, err)
	}

	nw.WriteHeader(200) // Should not panic or error
}

func TestOneShot(t *testing.T) {
	t.Run("successful write and fire", func(t *testing.T) {
		rec := httptest.NewRecorder()
		os := NewOneShot(rec)

		os.Header().Set("X-Custom-Header", "value123")
		os.WriteHeader(http.StatusCreated)

		n, err := os.Write([]byte("hello world"))
		if err != nil || n != 11 {
			t.Fatalf("Write() = (%d, %v), want (11, nil)", n, err)
		}

		if os.Status() != http.StatusCreated {
			t.Errorf("Status() = %d, want %d", os.Status(), http.StatusCreated)
		}

		err = os.Fire()
		if err != nil {
			t.Fatalf("unexpected Fire() error: %v", err)
		}

		res := rec.Result()
		if res.StatusCode != http.StatusCreated {
			t.Errorf("rec StatusCode = %d, want %d", res.StatusCode, http.StatusCreated)
		}
		if res.Header.Get("X-Custom-Header") != "value123" {
			t.Errorf("rec Header = %q, want 'value123'", res.Header.Get("X-Custom-Header"))
		}
		if rec.Body.String() != "hello world" {
			t.Errorf("rec Body = %q, want 'hello world'", rec.Body.String())
		}

		// Second Fire() returns ErrAlreadyFired
		err = os.Fire()
		if !errors.Is(err, ErrAlreadyFired) {
			t.Errorf("second Fire() got error %v, want %v", err, ErrAlreadyFired)
		}
	})

	t.Run("Reset method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		os := NewOneShot(rec)

		os.Header().Set("X-Foo", "Bar")
		_, _ = os.Write([]byte("some data"))

		os.Reset()

		if len(os.Header()) != 0 {
			t.Errorf("expected empty header after Reset, got %v", os.Header())
		}
	})
}
