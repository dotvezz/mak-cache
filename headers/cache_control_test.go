package headers

import (
	"reflect"
	"testing"
	"time"
)

func ptr[T any](v T) *T {
	return &v
}

func TestCacheControl_FromDirectives(t *testing.T) {
	tests := []struct {
		name     string
		ds       []string
		want     CacheControl
		wantErr  bool
	}{
		{
			name: "max-age",
			ds:   []string{"max-age=60"},
			want: CacheControl{MaxAge: ptr(60 * time.Second)},
		},
		{
			name: "stale-while-revalidate",
			ds:   []string{"stale-while-revalidate=30"},
			want: CacheControl{StaleWhileRevalidate: ptr(30 * time.Second)},
		},
		{
			name: "must-revalidate",
			ds:   []string{"must-revalidate"},
			want: CacheControl{MustRevalidate: true},
		},
		{
			name: "proxy-revalidate",
			ds:   []string{"proxy-revalidate"},
			want: CacheControl{ProxyRevalidate: true},
		},
		{
			name: "no-cache",
			ds:   []string{"no-cache"},
			want: CacheControl{NoCache: true},
		},
		{
			name: "no-store",
			ds:   []string{"no-store"},
			want: CacheControl{NoStore: true},
		},
		{
			name: "private",
			ds:   []string{"private"},
			want: CacheControl{Private: true},
		},
		{
			name: "public",
			ds:   []string{"public"},
			want: CacheControl{Private: false},
		},
		{
			name: "combined",
			ds:   []string{"public", "max-age=3600", "stale-while-revalidate=600", "must-revalidate"},
			want: CacheControl{
				MaxAge:               ptr(3600 * time.Second),
				StaleWhileRevalidate: ptr(600 * time.Second),
				MustRevalidate:       true,
				Private:              false,
			},
		},
		{
			name:    "invalid max-age",
			ds:      []string{"max-age=abc"},
			wantErr: true,
		},
		{
			name:    "invalid max-age no value",
			ds:      []string{"max-age"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CacheControl{}
			err := cc.FromDirectives(tt.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromDirectives() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(*cc, tt.want) {
				t.Errorf("FromDirectives() got = %v, want %v", *cc, tt.want)
			}
		})
	}
}

func TestCacheControl_Directives(t *testing.T) {
	tests := []struct {
		name string
		cc   CacheControl
		want []string
	}{
		{
			name: "max-age",
			cc:   CacheControl{MaxAge: ptr(60 * time.Second)},
			want: []string{"public", "max-age=60"},
		},
		{
			name: "stale-while-revalidate",
			cc:   CacheControl{StaleWhileRevalidate: ptr(30 * time.Second)},
			want: []string{"public", "stale-while-revalidate=30"},
		},
		{
			name: "private",
			cc:   CacheControl{Private: true},
			want: []string{"private"},
		},
		{
			name: "combined",
			cc: CacheControl{
				MaxAge:               ptr(3600 * time.Second),
				StaleWhileRevalidate: ptr(600 * time.Second),
				MustRevalidate:       true,
				NoCache:              true,
			},
			want: []string{"no-cache", "public", "max-age=3600", "stale-while-revalidate=600", "must-revalidate"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cc.Directives()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Directives() = %v, want %v", got, tt.want)
			}
		})
	}
}
