package headers

import (
	"testing"
	"time"
)

func TestCacheStatus_String(t *testing.T) {
	tests := []struct {
		name string
		cs   CacheStatus
		want string
	}{
		{
			name: "hit status",
			cs: CacheStatus{
				Hit:    true,
				TTL:    10 * time.Second,
				Key:    "abc123key",
				Stored: true,
			},
			want: "github.com/dotvezz/mak-cache; hit; ttl=10; key=abc123key; stored; key=abc123key",
		},
		{
			name: "forward uri-miss status",
			cs: CacheStatus{
				FwdURIMiss: true,
				FwdStatus:  200,
				Stored:     true,
				Key:        "mykey",
			},
			want: "github.com/dotvezz/mak-cache; fwd=uri-miss; fwd-status=200; stored; key=mykey",
		},
		{
			name: "forward vary-miss status",
			cs: CacheStatus{
				FwdVaryMiss: true,
			},
			want: "github.com/dotvezz/mak-cache; fwd=vary-miss",
		},
		{
			name: "forward request status",
			cs: CacheStatus{
				FwdRequest: true,
			},
			want: "github.com/dotvezz/mak-cache; fwd=request",
		},
		{
			name: "forward bypass status",
			cs: CacheStatus{
				FwdBypass: true,
			},
			want: "github.com/dotvezz/mak-cache; fwd=bypass",
		},
		{
			name: "forward method status",
			cs: CacheStatus{
				FwdMethod: true,
			},
			want: "github.com/dotvezz/mak-cache; fwd=method",
		},
		{
			name: "forward stale status",
			cs: CacheStatus{
				FwdStale: true,
			},
			want: "github.com/dotvezz/mak-cache; fwd=stale",
		},
		{
			name: "forward default miss status",
			cs:   CacheStatus{},
			want: "github.com/dotvezz/mak-cache; fwd=miss",
		},
		{
			name: "collapsed request with detail",
			cs: CacheStatus{
				Collapsed: true,
				FwdURIMiss: true,
				Detail:     "coalesced-singleflight",
			},
			want: "github.com/dotvezz/mak-cache; collapsed; fwd=uri-miss; detail=coalesced-singleflight",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cs.String()
			if got != tt.want {
				t.Errorf("CacheStatus.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
