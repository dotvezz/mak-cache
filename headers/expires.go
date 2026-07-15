package headers

import (
	"net/http"
	"time"
)

type Expires time.Time

func (e Expires) String() string {
	return time.Time(e).UTC().Format(http.TimeFormat)
}
