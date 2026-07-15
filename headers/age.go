package headers

import (
	"strconv"
	"time"
)

type Age time.Duration

func (a Age) String() string {
	return strconv.Itoa(int(time.Duration(a).Seconds()))
}
