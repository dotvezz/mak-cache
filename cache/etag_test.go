package cache

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"hash/crc32"
	"strconv"
	"testing"

	"github.com/dotvezz/caddy-cache/config"
)

func TestGenerateEtag(t *testing.T) {
	entry := &Entry{
		Body: []byte("hello world"),
	}

	t.Run("Disabled", func(t *testing.T) {
		cfg := config.ETagConfig{Disable: true}
		if got := GenerateEtag(entry, cfg); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("CRC32", func(t *testing.T) {
		cfg := config.ETagConfig{CRC32: true}
		expected := strconv.FormatUint(uint64(crc32.ChecksumIEEE(entry.Body)), 16)
		if got := GenerateEtag(entry, cfg); got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("SHA256", func(t *testing.T) {
		cfg := config.ETagConfig{SHA256: true}
		sum := sha256.Sum256(entry.Body)
		expected := hex.EncodeToString(sum[:])
		if got := GenerateEtag(entry, cfg); got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("MD5 (default)", func(t *testing.T) {
		cfg := config.ETagConfig{}
		sum := md5.Sum(entry.Body)
		expected := hex.EncodeToString(sum[:])
		if got := GenerateEtag(entry, cfg); got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})
}
