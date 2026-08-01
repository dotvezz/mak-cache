package cache

import (
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

type Entry struct {
	Metadata
	Status  int
	Body    []byte
	Headers [][2]string
}

func (e *Entry) GetHeader(k string) (v []string) {
	for _, h := range e.Headers {
		if h[0] == k {
			v = append(v, h[1])
		}
	}

	return v
}

func (e *Entry) FromResponse(rec caddyhttp.ResponseRecorder) {
	e.Status = rec.Status()
	h := rec.Header()
	for k := range h {
		for i := range h[k] {
			e.Headers = append(e.Headers, [2]string{k, h[k][i]})
		}
	}
}

// Metadata is metadata associated with a cache key
type Metadata struct {
	ETag         string
	Vary         []string
	CacheControl []string
	Date         time.Time
	Expires      time.Time
}
