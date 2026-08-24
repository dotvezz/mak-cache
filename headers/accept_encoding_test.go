package headers

import "testing"

func TestAcceptEncoding_String(t *testing.T) {
	tests := []struct {
		name string
		vals []string
		want string
	}{
		{
			name: "empty slice",
			vals: nil,
			want: "*",
		},
		{
			name: "single empty string",
			vals: []string{""},
			want: "*",
		},
		{
			name: "contains star",
			vals: []string{"gzip", "*", "br"},
			want: "*",
		},
		{
			name: "ranked encodings ordered properly",
			vals: []string{"gzip", "br", "zstd", "deflate"},
			want: "zstd, br, gzip, deflate",
		},
		{
			name: "identity encoding",
			vals: []string{"identity"},
			want: "identity",
		},
		{
			name: "unknown encoding ignored in ranking",
			vals: []string{"unknown-enc", "gzip"},
			want: "gzip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ae := AcceptEncoding{}
			ae.FromHeaders(tt.vals)
			got := ae.String()
			if got != tt.want {
				t.Errorf("AcceptEncoding.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
