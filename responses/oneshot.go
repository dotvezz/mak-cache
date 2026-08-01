package responses

import (
	"bytes"
	"errors"
	"net/http"
)

var (
	ErrAlreadyFired = errors.New("already fired")
)

func NewOneShot(inner http.ResponseWriter) *OneShot {
	return &OneShot{
		inner: inner,
		buf:   bytes.NewBuffer([]byte{}),
		Staged: http.Response{
			Header: make(http.Header),
		},
	}
}

type OneShot struct {
	inner http.ResponseWriter

	Staged http.Response

	buf *bytes.Buffer

	fired bool
}

func (o *OneShot) Reset() {
	o.Staged = http.Response{
		Header: make(http.Header)}
	o.buf.Reset()
}

func (o *OneShot) Fire() error {
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

func (o *OneShot) Header() http.Header {
	return o.Staged.Header
}

func (o *OneShot) Write(bytes []byte) (int, error) {
	return o.buf.Write(bytes)
}

func (o *OneShot) WriteHeader(statusCode int) {
	o.Staged.StatusCode = statusCode
}
