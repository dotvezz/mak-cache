package requests

import "net/http"

func IsSafeMethod(method string) bool {
	return method == http.MethodHead || method == http.MethodGet
}
