package wireinspect

import (
	"encoding/hex"
	"fmt"
)

const maxFieldNumber = 1<<29 - 1

// Wire types.
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireSGroup  = 3
	wireEGroup  = 4
	wireFixed32 = 5
)

// Record is one decoded field occurrence.
type Record struct {
	Path   string `json:"path"`
	Number uint32 `json:"number"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	Offset int    `json:"offset"`
	Value  any    `json:"value,omitempty"`
	// Unknown fields carry raw hex instead of a decoded value.
	Unknown bool   `json:"unknown,omitempty"`
	Wire    int    `json:"wire,omitempty"`
	Hex     string `json:"hex,omitempty"`
}

// DecodeError reports a failure with a byte offset relative to the
// start of the whole input message.
type DecodeError struct {
	Offset int
	Msg    string
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Msg)
}

func errf(off int, format string, args ...any) *DecodeError {
	return &DecodeError{Offset: off, Msg: fmt.Sprintf(format, args...)}
}

// Decode parses buf according to schema and returns one record per
// field occurrence, in wire order. Nested message values are []Record.
func Decode(buf []byte, schema *Schema) ([]Record, error) {
	d := &decoder{buf: buf}
	recs, err := d.message(0, len(buf), schema, "")
	if err != nil {
		return nil, err
	}
	return recs, nil
}

type decoder struct {
	buf []byte
}

func (d *decoder) varint(pos int) (uint64, int, error) {
	var v uint64
	for i := 0; i < 10; i++ {
		if pos+i >= len(d.buf) {
			return 0, 0, errf(pos, "truncated varint")
		}
		b := d.buf[pos+i]
		if i == 9 && b > 1 {
			return 0, 0, errf(pos, "varint overflows 64 bits")
		}
		v |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return v, pos + i + 1, nil
		}
	}
	return 0, 0, errf(pos, "varint overflows 64 bits")
}

func (d *decoder) message(start, end int, schema *Schema, path string) ([]Record, error) {
	var recs []Record
	pos := start
	for pos < end {
		tagOff := pos
		tag, next, err := d.varint(pos)
		if err != nil {
			return nil, err
		}
		num := uint32(tag >> 3)
		wire := int(tag & 7)
		if num == 0 || num > maxFieldNumber {
			return nil, errf(tagOff, "invalid field number %d", num)
		}
		switch wire {
		case wireVarint, wireFixed64, wireBytes, wireFixed32:
		case wireSGroup, wireEGroup:
			return nil, errf(tagOff, "group wire type %d not supported", wire)
		default:
			return nil, errf(tagOff, "invalid wire type %d", wire)
		}
		f := schema.fieldByNumber(num)
		if f == nil {
			raw, npos, err := d.skipUnknown(tagOff, next, wire, end)
			if err != nil {
				return nil, err
			}
			recs = append(recs, Record{
				Path:   joinPath(path, fmt.Sprintf("field_%d", num)),
				Number: num, Offset: tagOff, Unknown: true, Wire: wire,
				Hex: hex.EncodeToString(raw),
			})
			pos = npos
			continue
		}
		rec, npos, err := d.knownField(tagOff, next, f, wire, end, path)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec...)
		pos = npos
	}
	return recs, nil
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// skipUnknown consumes one unknown field value starting at pos and
// returns the raw bytes (tag excluded) up to end.
func (d *decoder) skipUnknown(tagOff, pos, wire, end int) ([]byte, int, error) {
	start := pos
	switch wire {
	case wireVarint:
		_, next, err := d.varint(pos)
		if err != nil {
			return nil, 0, err
		}
		return d.buf[start:next], next, nil
	case wireFixed64:
		if pos+8 > end {
			return nil, 0, errf(pos, "truncated fixed64")
		}
		return d.buf[start : pos+8], pos + 8, nil
	case wireFixed32:
		if pos+4 > end {
			return nil, 0, errf(pos, "truncated fixed32")
		}
		return d.buf[start : pos+4], pos + 4, nil
	case wireBytes:
		l, next, err := d.varint(pos)
		if err != nil {
			return nil, 0, err
		}
		if l > uint64(end-next) {
			return nil, 0, errf(pos, "length %d exceeds remaining %d bytes", l, end-next)
		}
		return d.buf[start : next+int(l)], next + int(l), nil
	}
	return nil, 0, errf(tagOff, "invalid wire type %d", wire)
}
