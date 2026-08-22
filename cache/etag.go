package cache

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"hash/crc32"
	"strconv"

	"github.com/dotvezz/mak-cache/config"
)

func GenerateEtag(e *Entry, cfg config.ETagConfig) string {
	if cfg.Disable {
		return ""
	}

	if cfg.CRC32 {
		return strconv.FormatUint(uint64(crc32.ChecksumIEEE(e.Body)), 16)
	}

	if cfg.SHA256 {
		sum := sha256.Sum256(e.Body)
		return hex.EncodeToString(sum[:])
	}

	sum := md5.Sum(e.Body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}
