package responses

import (
	"net/http"
	"time"
)

var now = time.Now

func NewTTFBWriter(rw http.ResponseWriter, start time.Time, observerFirstByte func(float64)) http.ResponseWriter {
	return &ttfbWriter{
		ResponseWriter:   rw,
		start:            start,
		observeFirstByte: observerFirstByte,
	}
}

type ttfbWriter struct {
	http.ResponseWriter

	start            time.Time
	firstByteSent    bool
	observeFirstByte func(float64)
}

func (w *ttfbWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *ttfbWriter) WriteHeader(statusCode int) {
	if !w.firstByteSent {
		w.firstByteSent = true
		w.observeFirstByte(now().Sub(w.start).Seconds())
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *ttfbWriter) Write(p []byte) (int, error) {
	if !w.firstByteSent {
		w.firstByteSent = true
		w.observeFirstByte(now().Sub(w.start).Seconds())
	}
	return w.ResponseWriter.Write(p)
}
