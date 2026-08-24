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
		name    string
		ds      []string
		want    CacheControl
		wantErr bool
	}{
		{
			name: "max-age",
			ds:   []string{"max-age=60"},
			want: CacheControl{MaxAge: ptr(60 * time.Second)},
		},
		{
			name: "min-fresh",
			ds:   []string{"min-fresh=15"},
			want: CacheControl{MinFresh: ptr(15 * time.Second)},
		},
		{
			name: "max-stale",
			ds:   []string{"max-stale=120"},
			want: CacheControl{MaxStale: ptr(120 * time.Second)},
		},
		{
			name: "s-maxage",
			ds:   []string{"s-maxage=180"},
			want: CacheControl{SMaxAge: ptr(180 * time.Second)},
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
			want: CacheControl{Public: true},
		},
		{
			name: "combined",
			ds:   []string{"public", "max-age=3600", "stale-while-revalidate=600", "must-revalidate"},
			want: CacheControl{
				MaxAge:               ptr(3600 * time.Second),
				StaleWhileRevalidate: ptr(600 * time.Second),
				MustRevalidate:       true,
				Public:               true,
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
		{
			name:    "invalid min-fresh no value",
			ds:      []string{"min-fresh"},
			wantErr: true,
		},
		{
			name:    "invalid min-fresh bad int",
			ds:      []string{"min-fresh=xyz"},
			wantErr: true,
		},
		{
			name:    "invalid max-stale no value",
			ds:      []string{"max-stale"},
			wantErr: true,
		},
		{
			name:    "invalid max-stale bad int",
			ds:      []string{"max-stale=xyz"},
			wantErr: true,
		},
		{
			name:    "invalid s-maxage no value",
			ds:      []string{"s-maxage"},
			wantErr: true,
		},
		{
			name:    "invalid s-maxage bad int",
			ds:      []string{"s-maxage=xyz"},
			wantErr: true,
		},
		{
			name:    "invalid stale-while-revalidate no value",
			ds:      []string{"stale-while-revalidate"},
			wantErr: true,
		},
		{
			name:    "invalid stale-while-revalidate bad int",
			ds:      []string{"stale-while-revalidate=xyz"},
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
			want: []string{"max-age=60"},
		},
		{
			name: "all directives",
			cc: CacheControl{
				NoCache:              true,
				NoStore:              true,
				Private:              true,
				Public:               true, // Private overrides public in Directives()
				MaxAge:               ptr(3600 * time.Second),
				MaxStale:             ptr(60 * time.Second),
				MinFresh:             ptr(30 * time.Second),
				SMaxAge:              ptr(1800 * time.Second),
				StaleWhileRevalidate: ptr(300 * time.Second),
				MustRevalidate:       true,
				ProxyRevalidate:      true,
			},
			want: []string{
				"no-cache",
				"no-store",
				"private",
				"max-age=3600",
				"max-stale=60",
				"min-fresh=30",
				"s-maxage=1800",
				"stale-while-revalidate=300",
				"must-revalidate",
				"proxy-revalidate",
			},
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

func TestCacheControl_StringAndFromString(t *testing.T) {
	cc := &CacheControl{
		Public: true,
		MaxAge: ptr(3600 * time.Second),
	}
	str := cc.String()
	wantStr := "public, max-age=3600"
	if str != wantStr {
		t.Errorf("String() = %q, want %q", str, wantStr)
	}

	var parsed CacheControl
	err := parsed.FromString(wantStr)
	if err != nil {
		t.Fatalf("FromString() error = %v", err)
	}
	if !parsed.Public || *parsed.MaxAge != 3600*time.Second {
		t.Errorf("FromString() got %+v", parsed)
	}
}

func TestCacheControl_Cacheable(t *testing.T) {
	t.Run("Response cacheability", func(t *testing.T) {
		publicCC := CacheControl{Public: true}
		if !publicCC.Cacheable(true) {
			t.Error("expected public response to be cacheable")
		}

		noStoreCC := CacheControl{NoStore: true}
		if noStoreCC.Cacheable(true) {
			t.Error("expected no-store response to be uncacheable")
		}

		privateCC := CacheControl{Private: true}
		if privateCC.Cacheable(true) {
			t.Error("expected private response to be uncacheable")
		}
	})

	t.Run("Request cacheability", func(t *testing.T) {
		normalCC := CacheControl{}
		if !normalCC.Cacheable(false) {
			t.Error("expected normal request to be cacheable")
		}

		noStoreCC := CacheControl{NoStore: true}
		if noStoreCC.Cacheable(false) {
			t.Error("expected no-store request to be uncacheable")
		}

		noCacheCC := CacheControl{NoCache: true}
		if noCacheCC.Cacheable(false) {
			t.Error("expected no-cache request to be uncacheable")
		}
	})
}
