package cache

import (
	"net/http"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

var (
	entryBaseHeapSize = int(unsafe.Sizeof(Entry{}))
	metaBaseHeapSize  = int(unsafe.Sizeof(Metadata{}))
)

type Entry struct {
	Metadata
	Status  int
	Body    []byte
	Headers [][2]string
}

func (e *Entry) RefreshHeapSize() {

}

func (e *Entry) HeapSize() int {
	total := entryBaseHeapSize

	// embedded metadata size
	total += e.Metadata.HeapSize()

	// body size
	total += sizeClass(cap(e.Body))

	// headers
	if e.Headers != nil {
		// backing array
		total += cap(e.Headers) * int(unsafe.Sizeof([2]string{}))

		for i := range e.Headers {
			total += sizeClass(len(e.Headers[i][0]))
			total += sizeClass(len(e.Headers[i][1]))
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
	Evict             time.Time
	Linked            []string
	NeedsRevalidation bool
}

func (m *Metadata) EvictAt() time.Time {
	return m.Evict
}

func (m *Metadata) RefreshHeapSize() {
	total := metaBaseHeapSize

	total += sizeClass(len(m.ETag))
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
		total += sizeClass(len(s[i]))
	}

	return total
}
