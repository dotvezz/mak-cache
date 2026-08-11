package cache

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

var (
	entrySignature    = [4]byte{'e', 'n', 't', 'r'}
	metadataSignature = [4]byte{'m', 'e', 't', 'a'}
)

const (
	entryV1 uint8 = 1
	metaV1  uint8 = 1
)

type Writer interface {
	io.Writer
	io.StringWriter
	io.ByteWriter
}

type Reader interface {
	io.Reader
	io.ByteReader
}

func checkSignature(r Reader, signature [4]byte) error {
	b4 := [4]byte{}
	err := binary.Read(r, binary.BigEndian, &b4)
	if err != nil {
		return fmt.Errorf("error reading signature byte: %w", err)
	}
	if b4 != signature {
		return fmt.Errorf("invalid signature byte: %v", b4)
	}

	return nil
}

func checkVersion(r Reader, v byte) (err error) {
	var b byte
	b, err = r.ReadByte()
	if err != nil {
		return err
	}
	if b != v {
		return fmt.Errorf("unsupported version %d", b)
	}
	return nil
}

func encodeHeaders(buf Writer, headers [][2]string) (err error) {
	err = binary.Write(buf, binary.BigEndian, uint16(len(headers)))
	if err != nil {
		return err
	}

	for i := range headers {
		err = encodeString(buf, headers[i][0])
		if err != nil {
			return err
		}
		err = encodeString(buf, headers[i][1])
		if err != nil {
			return err
		}
	}
	return err
}

func encodeString(buf Writer, s string) (err error) {
	err = binary.Write(buf, binary.BigEndian, uint16(len(s)))
	if err != nil {
		return err
	}
	_, err = buf.WriteString(s)
	return err
}

func encodeStringSlice(buf Writer, ss []string) (err error) {
	err = binary.Write(buf, binary.BigEndian, uint16(len(ss)))
	if err != nil {
		return err
	}
	for i := range ss {
		err = encodeString(buf, ss[i])
		if err != nil {
			return err
		}
	}
	return err
}

func encodeBool(buf Writer, b bool) (err error) {
	var i uint8
	if b {
		i = 1
	}
	err = binary.Write(buf, binary.BigEndian, i)
	return err
}

func decodeHeaders(r Reader) (h [][2]string, err error) {
	var l uint16
	err = binary.Read(r, binary.BigEndian, &l)
	if err != nil {
		return h, err
	}
	h = make([][2]string, l)
	for i := range h {
		h[i][0], err = decodeString(r)
		if err != nil {
			return h, err
		}
		h[i][1], err = decodeString(r)
		if err != nil {
			return h, err
		}
	}
	return h, err
}

func decodeStringSlice(r Reader) (s []string, err error) {
	var l uint16
	err = binary.Read(r, binary.BigEndian, &l)
	if err != nil {
		return s, err
	}
	s = make([]string, l)
	for i := range l {
		s[i], err = decodeString(r)
		if err != nil {
			return s, err
		}
	}

	return s, err
}

func decodeString(r Reader) (_ string, err error) {
	var l uint16
	err = binary.Read(r, binary.BigEndian, &l)
	if err != nil {
		return "", err
	}
	v := make([]byte, l)
	_, err = r.Read(v)
	return string(v), err
}

func decodeTime(r Reader) (_ time.Time, err error) {
	var i int64
	err = binary.Read(r, binary.BigEndian, &i)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(i), nil
}

func decodeBool(r Reader) (_ bool, err error) {
	var i uint
	err = binary.Read(r, binary.BigEndian, &i)
	if err != nil {
		return false, err
	}
	return i > 0, nil
}

func (e *Entry) Size() int {
	var total = 8 + // 8 bytes for the Status int
		24 + // 24 bytes for the body slice header
		24 // 24 bytes for the header slice header

	// embedded metadata size
	total += e.Metadata.Size()

	// body size
	total += cap(e.Body)

	// headers
	if cap(e.Headers) > 0 {
		total += cap(e.Headers) * 32 // 32 bytes for each element
		for i := range e.Headers {
			total += len(e.Headers[i][0])
			total += len(e.Headers[i][1])
		}
	}

	return total
}

func (e *Entry) MarshalTo(buf Writer) (err error) {
	err = binary.Write(buf, binary.BigEndian, entrySignature)
	if err != nil {
		return err
	}
	err = buf.WriteByte(entryV1)
	if err != nil {
		return err
	}

	err = e.Metadata.MarshalTo(buf)
	if err != nil {
		return err
	}

	err = encodeHeaders(buf, e.Headers)
	if err != nil {
		return err
	}

	err = binary.Write(buf, binary.BigEndian, uint16(e.Status))
	if err != nil {
		return err
	}
	err = binary.Write(buf, binary.BigEndian, uint64(len(e.Body)))
	if err != nil {
		return err
	}
	_, err = buf.Write(e.Body)
	return err
}

func (e *Entry) Unmarshal(r Reader) (err error) {
	if err = checkSignature(r, entrySignature); err != nil {
		return err
	}

	if err = checkVersion(r, entryV1); err != nil {
		return err
	}

	err = e.Metadata.Unmarshal(r)
	if err != nil {
		return err
	}

	e.Headers, err = decodeHeaders(r)
	if err != nil {
		return err
	}

	{
		var status uint16
		err = binary.Read(r, binary.BigEndian, &status)
		if err != nil {
			return err
		}
		e.Status = int(status)
	}

	{
		var lenBody uint64
		err = binary.Read(r, binary.BigEndian, &lenBody)
		if err != nil {
			return err
		}

		e.Body = make([]byte, lenBody)
		_, err = r.Read(e.Body)
		if err != nil {
			return err
		}
	}

	return err
}

func (e *Metadata) Size() int {
	var total = 16 + // 16 bytes for the ETag string header
		24 + // 24 bytes for the Vary slice header
		24 + // 24 bytes for the CacheControl slice header
		24 + // 24 bytes for the ttl time.Time
		1 // 1 byte for NeedsRevalidation

	total += len(e.ETag)

	for i := range e.Vary {
		// 16 for header, plus the string length
		total += 16 + len(e.Vary[i])
	}
	for i := range e.CacheControl {
		// 16 for header, plus the string length
		total += 16 + len(e.CacheControl[i])
	}

	return total
}

func (m *Metadata) MarshalTo(buf Writer) (err error) {
	err = binary.Write(buf, binary.BigEndian, metadataSignature)
	if err != nil {
		return err
	}
	err = buf.WriteByte(metaV1)
	if err != nil {
		return err
	}

	err = encodeString(buf, m.ETag)
	if err != nil {
		return err
	}
	err = encodeStringSlice(buf, m.Vary)
	if err != nil {
		return err
	}
	err = encodeStringSlice(buf, m.CacheControl)
	if err != nil {
		return err
	}

	err = binary.Write(buf, binary.BigEndian, m.Expires.UnixMilli())
	if err != nil {
		return err
	}
	err = binary.Write(buf, binary.BigEndian, m.Date.UnixMilli())
	if err != nil {
		return err
	}
	err = encodeBool(buf, m.NeedsRevalidation)
	if err != nil {
		return err
	}
	err = encodeStringSlice(buf, m.Linked)
	if err != nil {
		return err
	}

	return err
}

func (m *Metadata) Unmarshal(r Reader) (err error) {
	if err = checkSignature(r, metadataSignature); err != nil {
		return err
	}

	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	if b != metaV1 {
		return fmt.Errorf("unsupported version %d", b)
	}

	m.ETag, err = decodeString(r)
	if err != nil {
		return err
	}
	m.Vary, err = decodeStringSlice(r)
	if err != nil {
		return err
	}
	m.CacheControl, err = decodeStringSlice(r)
	if err != nil {
		return err
	}
	m.Expires, err = decodeTime(r)
	if err != nil {
		return err
	}
	m.Date, err = decodeTime(r)
	if err != nil {
		return err
	}
	m.NeedsRevalidation, err = decodeBool(r)
	if err != nil {
		return err
	}
	m.Linked, err = decodeStringSlice(r)
	if err != nil {
		return err
	}

	return err
}
