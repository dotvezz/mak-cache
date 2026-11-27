package responses

import (
	"bytes"
	"errors"
	"net/http"
)

var (
	ErrAlreadyFired = errors.New("already fired")
)

// NewOneShot creates a new OneShotWriter that wraps the provided http.ResponseWriter.
func NewOneShot(inner http.ResponseWriter) *OneShotWriter {
	return &OneShotWriter{
		inner: inner,
		buf:   bytes.NewBuffer([]byte{}),
		Staged: http.Response{
			Header:     make(http.Header),
			StatusCode: http.StatusBadGateway, // Default to 502 Bad Gateway for early-fire in error cases
		},
	}
}

// OneShotWriter is a wrapper around http.ResponseWriter that allows for staging a response before firing it.
// The response can be reset and overwritten multiple times before firing. This allows us to collect responses from
// origins and hold, inspect, and conditionally handle them before writing to the client.
type OneShotWriter struct {
	inner http.ResponseWriter

	Staged http.Response

	buf *bytes.Buffer

	fired bool
}

func (o *OneShotWriter) Status() int {
	return o.Staged.StatusCode
}

func (o *OneShotWriter) Reset() {
	o.Staged = http.Response{
		Header: make(http.Header)}
	o.buf.Reset()
}

func (o *OneShotWriter) Fire() error {
	if o.fired {
		return ErrAlreadyFired
	}

	o.fired = true

	for k := range o.Staged.Header {
		o.inner.Header().Add(k, o.Staged.Header.Get(k))
	}

	o.inner.WriteHeader(o.Staged.StatusCode)

	_, err := o.buf.WriteTo(o.inner)
	return err
}

func (o *OneShotWriter) Header() http.Header {
	return o.Staged.Header
}

func (o *OneShotWriter) Write(bytes []byte) (int, error) {
	return o.buf.Write(bytes)
}

func (o *OneShotWriter) WriteHeader(statusCode int) {
	o.Staged.StatusCode = statusCode
}
