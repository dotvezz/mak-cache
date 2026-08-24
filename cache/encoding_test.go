package cache

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"
)

var (
	basicEntry = Entry{
		Metadata: Metadata{
			ETag:         `"12345"`,
			Vary:         []string{"Accept-Encoding", "User-Agent"},
			CacheControl: []string{"max-age=3600", "public"},
			Expires:      time.Unix(1700000000, 0),
		},
		Status: 200,
		Body:   []byte("Hello, world!"),
		Headers: [][2]string{
			{"Content-Type", "text/plain"},
			{"X-Whatever", "Thing1"},
		},
	}

	bigEntry = Entry{
		Metadata: Metadata{
			ETag:         `"big-etag-67890"`,
			Vary:         []string{"Accept-Encoding"},
			CacheControl: []string{"max-age=600"},
			Expires:      time.Unix(1700003600, 0),
		},
		Status: 200,
		Body:   bigJSON,
		Headers: [][2]string{
			{"Content-Type", "application/json"},
			{"Cache-Control", "max-age=600"},
			{"X-Cache", "HIT"},
			{"Content-Length", strconv.Itoa(len(bigJSON))},
		},
	}
)

func TestEntry_MarshalUnmarshal(t *testing.T) {
	type test struct {
		name  string
		entry Entry
	}
	tests := []test{
		{
			name:  "Simple",
			entry: basicEntry,
		},
		{
			name:  "Big",
			entry: bigEntry,
		},
		{
			name: "Empty Body",
			entry: Entry{
				Status:  204,
				Body:    []byte{},
				Headers: [][2]string{},
			},
		},
		{
			name: "Zero",
			entry: Entry{
				Status:  200,
				Body:    []byte("0"),
				Headers: [][2]string{},
			},
		},
		{
			name: "Header with empty string",
			entry: Entry{
				Status: 200,
				Body:   []byte("0"),
				Headers: [][2]string{
					{"X-Whatever", ""},
				},
			},
		},
		{
			name: "Header with multiple values",
			entry: Entry{
				Status: 200,
				Body:   []byte("Hello, World"),
				Headers: [][2]string{
					{"X-Whatever", "Thing"},
					{"X-Whatever", "Thing2"},
				},
			},
		},
	}

	for _, tt := range tests {
		buf := bytes.NewBuffer(make([]byte, 0, 1024))
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			err := tt.entry.MarshalTo(buf)
			if err != nil {
				t.Fatal(err)
			}

			var entry = Entry{}
			err = entry.Unmarshal(buf)
			if err != nil {
				t.Fatal(err)
			}

			if entry.Status != tt.entry.Status {
				t.Fatal("Status mismatch")
			}

			if !bytes.Equal(entry.Body, tt.entry.Body) {
				t.Fatal("Body mismatch")
			}

			if !slices.Equal(entry.Headers, tt.entry.Headers) {
				t.Fatalf("Headers mismatch")
			}

			if entry.ETag != tt.entry.ETag {
				t.Fatal("ETag mismatch")
			}

			if !slices.Equal(entry.Vary, tt.entry.Vary) {
				t.Fatal("Vary mismatch")
			}

			if !slices.Equal(entry.CacheControl, tt.entry.CacheControl) {
				t.Fatal("CacheControl mismatch")
			}

			if !entry.Expires.Equal(tt.entry.Expires) {
				t.Fatalf("Expires mismatch: got %v, want %v", entry.Expires, tt.entry.Expires)
			}
		})
	}
}

func TestMetadata_MarshalUnmarshal(t *testing.T) {
	type test struct {
		name     string
		metadata Metadata
	}
	tests := []test{
		{
			name: "Basic Metadata",
			metadata: Metadata{
				ETag:         `"etag-xyz"`,
				Vary:         []string{"Accept-Encoding", "Accept-Language"},
				CacheControl: []string{"public", "max-age=86400"},
				Expires:      time.Unix(1700000000, 0),
			},
		},
		{
			name:     "Empty Metadata",
			metadata: Metadata{},
		},
	}

	for _, tt := range tests {
		buf := bytes.NewBuffer(make([]byte, 0, 1024))

		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			err := tt.metadata.MarshalTo(buf)
			if err != nil {
				t.Fatal(err)
			}

			var meta Metadata
			err = meta.Unmarshal(buf)
			if err != nil {
				t.Fatal(err)
			}

			if meta.ETag != tt.metadata.ETag {
				t.Fatal("ETag mismatch")
			}

			if !slices.Equal(meta.Vary, tt.metadata.Vary) {
				t.Fatal("Vary mismatch")
			}

			if !slices.Equal(meta.CacheControl, tt.metadata.CacheControl) {
				t.Fatal("CacheControl mismatch")
			}

			if !meta.Expires.Equal(tt.metadata.Expires) {
				t.Fatalf("Expires mismatch: got %v, want %v", meta.Expires, tt.metadata.Expires)
			}
		})
	}
}

func TestUnmarshal_ErrorPaths(t *testing.T) {
	t.Run("Entry invalid signature", func(t *testing.T) {
		r := bytes.NewReader([]byte("bad_signature"))
		var e Entry
		err := e.Unmarshal(r)
		if err == nil {
			t.Error("expected error for invalid signature, got nil")
		}
	})

	t.Run("Entry invalid version", func(t *testing.T) {
		buf := bytes.NewBuffer(nil)
		buf.Write(entrySignature[:])
		buf.WriteByte(99) // unsupported version 99
		var e Entry
		err := e.Unmarshal(buf)
		if err == nil {
			t.Error("expected error for invalid version, got nil")
		}
	})

	t.Run("Metadata invalid signature", func(t *testing.T) {
		r := bytes.NewReader([]byte("bad_signature"))
		var m Metadata
		err := m.Unmarshal(r)
		if err == nil {
			t.Error("expected error for invalid signature, got nil")
		}
	})

	t.Run("Metadata invalid version", func(t *testing.T) {
		buf := bytes.NewBuffer(nil)
		buf.Write(metadataSignature[:])
		buf.WriteByte(99) // unsupported version 99
		var m Metadata
		err := m.Unmarshal(buf)
		if err == nil {
			t.Error("expected error for invalid version, got nil")
		}
	})

	t.Run("Truncated Entry header unmarshaling", func(t *testing.T) {
		buf := bytes.NewBuffer(nil)
		_ = basicEntry.MarshalTo(buf)
		fullBytes := buf.Bytes()

		for i := 1; i <= 20; i++ {
			var e Entry
			r := bytes.NewReader(fullBytes[:i])
			err := e.Unmarshal(r)
			if err == nil {
				t.Errorf("expected error when unmarshaling truncated header slice of length %d", i)
			}
		}
	})

	t.Run("Truncated Metadata unmarshaling", func(t *testing.T) {
		buf := bytes.NewBuffer(nil)
		_ = basicEntry.Metadata.MarshalTo(buf)
		fullBytes := buf.Bytes()

		for i := 1; i < len(fullBytes); i++ {
			var m Metadata
			r := bytes.NewReader(fullBytes[:i])
			err := m.Unmarshal(r)
			if err == nil {
				t.Errorf("expected error when unmarshaling truncated metadata slice of length %d/%d", i, len(fullBytes))
			}
		}
	})
}

func Benchmark_SmallEntry_Marshal(b *testing.B) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	for b.Loop() {
		buf.Reset()
		err := basicEntry.MarshalTo(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_SmallEntry_Unmarshal(b *testing.B) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))

	err := basicEntry.MarshalTo(buf)
	if err != nil {
		b.Fatal(err)
	}
	bs := buf.Bytes()

	for b.Loop() {
		buf.Reset()
		rsp := Entry{}
		r := bytes.NewReader(bs)
		err = rsp.Unmarshal(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_SmallEntry_JSON_Marshal(b *testing.B) {
	for b.Loop() {
		_, err := json.Marshal(basicEntry)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_LargeEntry_Marshal(b *testing.B) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))

	for b.Loop() {
		buf.Reset()
		err := bigEntry.MarshalTo(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_LargeEntry_Unmarshal(b *testing.B) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))

	err := bigEntry.MarshalTo(buf)
	if err != nil {
		b.Fatal(err)
	}
	bs := buf.Bytes()

	for b.Loop() {
		buf.Reset()
		rsp := Entry{}
		r := bytes.NewReader(bs)
		err = rsp.Unmarshal(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzEntry_All(f *testing.F) {
	f.Add(bigJSON, "Test-Header", "Test Header Value", uint16(200), 1)
	buf := bytes.NewBuffer(make([]byte, 0, 1024))

	f.Fuzz(func(t *testing.T, body []byte, headerName, headerVal string, status uint16, headerCount int) {
		buf.Reset()
		if headerCount < 0 || headerCount > 100 {
			return
		}
		r := Entry{
			Body:    body,
			Status:  int(status),
			Headers: make([][2]string, 0, headerCount),
		}

		for range headerCount {
			r.Headers = append(r.Headers, [2]string{headerName, headerVal})
		}

		err := r.MarshalTo(buf)
		if err != nil {
			t.Fatal(err)
		}

		var entry = Entry{}
		err = entry.Unmarshal(buf)
		if err != nil {
			t.Fatal(err)
		}
		if entry.Status != r.Status {
			t.Fatal("Status mismatch")
		}

		if !bytes.Equal(entry.Body, r.Body) {
			t.Fatal("Body mismatch")
		}

		if !slices.Equal(entry.Headers, r.Headers) {
			t.Fatalf("Headers mismatch")
		}
	})
}

func FuzzEntry_Body(f *testing.F) {
	f.Add([]byte("Test Body"))
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	f.Fuzz(func(t *testing.T, body []byte) {
		buf.Reset()
		r := basicEntry
		r.Body = body
		err := r.MarshalTo(buf)
		if err != nil {
			t.Fatal(err)
		}

		var entry = Entry{}
		err = entry.Unmarshal(buf)
		if err != nil {
			t.Fatal(err)
		}
		if entry.Status != r.Status {
			t.Fatal("Status mismatch")
		}

		if !bytes.Equal(entry.Body, r.Body) {
			t.Fatal("Body mismatch")
		}

		if !slices.Equal(entry.Headers, r.Headers) {
			t.Fatalf("Headers mismatch")
		}
	})
}

func FuzzEntry_HeaderName(f *testing.F) {
	f.Add("Test-Header")
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	f.Fuzz(func(t *testing.T, headerName string) {
		buf.Reset()
		r := basicEntry
		r.Headers = make([][2]string, 1)
		r.Headers[0] = [2]string{headerName, "test"}

		err := r.MarshalTo(buf)
		if err != nil {
			t.Fatal(err)
		}

		var entry = Entry{}
		err = entry.Unmarshal(buf)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Status != r.Status {
			t.Fatal("Status mismatch")
		}

		if !bytes.Equal(entry.Body, r.Body) {
			t.Fatal("Body mismatch")
		}

		if !slices.Equal(entry.Headers, r.Headers) {
			t.Fatalf("Headers mismatch")
		}
	})
}

func FuzzEntry_HeaderVal(f *testing.F) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	f.Add("Test Header Value")
	f.Fuzz(func(t *testing.T, val string) {
		buf.Reset()

		r := basicEntry
		r.Headers = make([][2]string, 1)
		r.Headers[0] = [2]string{"X-Test", val}

		err := r.MarshalTo(buf)
		if err != nil {
			t.Fatal(err)
		}

		var entry = Entry{}
		err = entry.Unmarshal(buf)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Status != r.Status {
			t.Fatal("Status mismatch")
		}

		if !bytes.Equal(entry.Body, r.Body) {
			t.Fatal("Body mismatch")
		}

		if !slices.Equal(entry.Headers, r.Headers) {
			t.Fatalf("Headers mismatch")
		}
	})
}
