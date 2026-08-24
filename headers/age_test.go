package headers

import (
	"testing"
	"time"
)

func TestAge_String(t *testing.T) {
	tests := []struct {
		name string
		age  Age
		want string
	}{
		{
			name: "zero age",
			age:  Age(0),
			want: "0",
		},
		{
			name: "5 seconds",
			age:  Age(5 * time.Second),
			want: "5",
		},
		{
			name: "1 hour (3600s)",
			age:  Age(time.Hour),
			want: "3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.age.String()
			if got != tt.want {
				t.Errorf("Age.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
