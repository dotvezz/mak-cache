package requests

import (
	"net/http"
	"testing"
)

func TestIsSafeMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{http.MethodGet, true},
		{http.MethodHead, true},
		{http.MethodPost, false},
		{http.MethodPut, false},
		{http.MethodDelete, false},
		{http.MethodPatch, false},
		{http.MethodOptions, false},
	}

	for _, tt := range tests {
		got := IsSafeMethod(tt.method)
		if got != tt.want {
			t.Errorf("IsSafeMethod(%q) = %v, want %v", tt.method, got, tt.want)
		}
	}
}
