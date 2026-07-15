package responses

import "net/http"

type NoopWriter struct{}

func (NoopWriter) Header() http.Header {
	return http.Header{}
}

func (NoopWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (NoopWriter) WriteHeader(_ int) {

}
