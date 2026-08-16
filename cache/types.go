package cache

import (
	"net/http"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

type Entry struct {
	Metadata
	Status  int
	Body    []byte
	Headers [][2]string
}

func (e *Entry) HeapSize() int {
	var total = 8 + // 8 bytes for the Status int
		24 + // 24 bytes for the body slice header
		24 // 24 bytes for the header slice header

	// embedded metadata size
	total += e.Metadata.HeapSize()

	// body size
	total += cap(e.Body)

	// headers
	if cap(e.Headers) > 0 {
		total += cap(e.Headers) * 32 // 32 bytes for each element
		for i := range e.Headers {
			total += len(e.Headers[i][0])
			total += len(e.Headers[i][1])
		}
	}

	return total
}

func (e *Entry) GetHeader(k string) (v []string) {
	k = http.CanonicalHeaderKey(k)
	for _, h := range e.Headers {
		if h[0] == k {
			v = append(v, h[1])
		}
	}

	return v
}

func (e *Entry) FromResponse(rec caddyhttp.ResponseRecorder) {
	e.Status = rec.Status()
	e.Body = rec.Buffer().Bytes()
	h := rec.Header()
	e.Headers = [][2]string{}
	for k := range h {
		for i := range h[k] {
			e.Headers = append(e.Headers, [2]string{k, h[k][i]})
		}
	}
}

// Metadata is metadata associated with a cache key
type Metadata struct {
	heapSize          atomic.Int64
	ETag              string
	Vary              []string
	CacheControl      []string
	Date              time.Time
	Expires           time.Time
	NeedsRevalidation bool
	Linked            []string
}

var metaBaseHeapSize = int(unsafe.Sizeof(Metadata{}))

func (m *Metadata) RefreshHeapSize() {
	total := metaBaseHeapSize

	total += len(m.ETag)

	total += stringsHeapSize(m.Vary)
	total += stringsHeapSize(m.CacheControl)
	total += stringsHeapSize(m.Linked)

	m.heapSize.Store(int64(total))
}

func (m *Metadata) HeapSize() int {
	return int(m.heapSize.Load())
}

func stringsHeapSize(s []string) int {
	if s == nil {
		return 0
	}

	total := cap(s) * int(unsafe.Sizeof(""))

	for i := range s {
		total += len(s[i])
	}

	return total
}
