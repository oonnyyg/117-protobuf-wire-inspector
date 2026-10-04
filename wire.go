package main

import (
	"encoding/binary"
	"fmt"
)

const (
	WireVarint          = 0
	WireFixed64         = 1
	WireLengthDelimited = 2
	WireFixed32         = 5
)

const maxFieldNumber = 1<<29 - 1

// DecodeError reports a failure at a byte offset relative to the start of
// the whole input message.
type DecodeError struct {
	Offset int
	Msg    string
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Msg)
}

func errAt(offset int, format string, args ...any) *DecodeError {
	return &DecodeError{Offset: offset, Msg: fmt.Sprintf(format, args...)}
}

// readVarint reads a base-128 varint from buf starting at pos. It rejects
// truncated input and values that overflow 64 bits.
func readVarint(buf []byte, pos int) (uint64, int, error) {
	var value uint64
	for i := 0; i < 10; i++ {
		if pos+i >= len(buf) {
			return 0, 0, errAt(pos, "truncated varint")
		}
		b := buf[pos+i]
		if i == 9 && b > 1 {
			return 0, 0, errAt(pos, "varint overflows 64 bits")
		}
		value |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return value, pos + i + 1, nil
		}
	}
	return 0, 0, errAt(pos, "varint overflows 64 bits")
}

// readTag reads a field tag and returns the field number, wire type and the
// position after the tag.
func readTag(buf []byte, pos int) (int, int, int, error) {
	tag, next, err := readVarint(buf, pos)
	if err != nil {
		return 0, 0, 0, err
	}
	field := int(tag >> 3)
	wire := int(tag & 0x7)
	if field == 0 {
		return 0, 0, 0, errAt(pos, "field number 0 is not allowed")
	}
	if field > maxFieldNumber {
		return 0, 0, 0, errAt(pos, "field number %d exceeds 29-bit maximum", field)
	}
	switch wire {
	case WireVarint, WireFixed64, WireLengthDelimited, WireFixed32:
	default:
		return 0, 0, 0, errAt(pos, "invalid wire type %d", wire)
	}
	return field, wire, next, nil
}

// readFixed32 reads a 4-byte little-endian value.
func readFixed32(buf []byte, pos int) (uint32, int, error) {
	if pos+4 > len(buf) {
		return 0, 0, errAt(pos, "truncated fixed32")
	}
	return binary.LittleEndian.Uint32(buf[pos:]), pos + 4, nil
}

// zigzagDecode converts a ZigZag-encoded uint64 to int64.
func zigzagDecode(v uint64) int64 {
	return int64(v>>1) ^ -int64(v&1)
}
