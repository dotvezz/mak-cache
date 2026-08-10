package headers

import (
	"net/http"
	"time"
)

type Expires time.Time

func (e Expires) String() string {
	return time.Time(e).UTC().Format(http.TimeFormat)
}

func (e *Expires) FromString(v string) error {
	t, err := time.Parse(http.TimeFormat, v)
	if err == nil {
		*e = Expires(t)
	}
	return err
}
