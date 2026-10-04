package main

import (
	"encoding/hex"
	"unicode/utf8"
)

// Record is one decoded field occurrence, in wire order.
type Record struct {
	Offset int    `json:"offset"`
	Field  int    `json:"field"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	Value  any    `json:"value,omitempty"`
	// Unknown fields keep their wire type and raw bytes.
	Unknown  bool   `json:"unknown,omitempty"`
	WireType int    `json:"wire_type,omitempty"`
	Hex      string `json:"hex,omitempty"`
}

// DecodeMessage decodes buf[base:end] as a message described by desc.
// Offsets in records and errors are relative to the start of buf, i.e. the
// whole top-level packet.
func DecodeMessage(buf []byte, base, end int, desc *MessageDesc) ([]Record, error) {
	var records []Record
	pos := base
	for pos < end {
		tagStart := pos
		field, wire, next, err := readTag(buf[:end], pos)
		if err != nil {
			return nil, err
		}
		pos = next
		fd := desc.ByNumber[field]
		if fd == nil {
			rawEnd, err := skipUnknown(buf[:end], pos, wire, tagStart)
			if err != nil {
				return nil, err
			}
			records = append(records, Record{
				Offset:   tagStart,
				Field:    field,
				Unknown:  true,
				WireType: wire,
				Hex:      hex.EncodeToString(buf[tagStart:rawEnd]),
			})
			pos = rawEnd
			continue
		}
		recs, newPos, err := decodeKnown(buf, end, tagStart, pos, fd, wire)
		if err != nil {
			return nil, err
		}
		records = append(records, recs...)
		pos = newPos
	}
	return records, nil
}

// decodeKnown decodes one occurrence of a declared field. Packed repeated
// integers expand to one record per element.
func decodeKnown(buf []byte, end, tagStart, pos int, fd *FieldDesc, wire int) ([]Record, int, error) {
	canonical := fd.wireType()
	if fd.Repeated && fd.packable() && wire == WireLengthDelimited {
		return decodePacked(buf, end, tagStart, pos, fd)
	}
	if wire != canonical {
		return nil, 0, errAt(tagStart, "field %d (%s): wire type %d does not match declared type %s (wire type %d)",
			fd.Number, fd.Name, wire, fd.Type, canonical)
	}
	rec, next, err := decodeScalar(buf, end, tagStart, pos, fd)
	if err != nil {
		return nil, 0, err
	}
	return []Record{rec}, next, nil
}

func decodeScalar(buf []byte, end, tagStart, pos int, fd *FieldDesc) (Record, int, error) {
	rec := Record{Offset: tagStart, Field: fd.Number, Name: fd.Name, Type: string(fd.Type)}
	switch fd.Type {
	case TypeUint64:
		v, next, err := readVarint(buf[:end], pos)
		if err != nil {
			return rec, 0, err
		}
		rec.Value = v
		return rec, next, nil
	case TypeSint64:
		v, next, err := readVarint(buf[:end], pos)
		if err != nil {
			return rec, 0, err
		}
		rec.Value = zigzagDecode(v)
		return rec, next, nil
	case TypeFixed32:
		v, next, err := readFixed32(buf[:end], pos)
		if err != nil {
			return rec, 0, err
		}
		rec.Value = v
		return rec, next, nil
	case TypeString, TypeBytes, TypeMessage:
		data, next, err := readLengthDelimited(buf, end, pos)
		if err != nil {
			return rec, 0, err
		}
		switch fd.Type {
		case TypeString:
			if !utf8.Valid(data) {
				return rec, 0, errAt(pos, "field %d (%s): invalid UTF-8 string", fd.Number, fd.Name)
			}
			rec.Value = string(data)
		case TypeBytes:
			rec.Value = hex.EncodeToString(data)
		case TypeMessage:
			sub, err := DecodeMessage(buf, next-len(data), next, fd.Message)
			if err != nil {
				return rec, 0, err
			}
			rec.Value = sub
		}
		return rec, next, nil
	}
	return rec, 0, errAt(tagStart, "unsupported type %q", fd.Type)
}

// decodePacked expands a packed repeated integer field into one record per
// element, preserving order.
func decodePacked(buf []byte, end, tagStart, pos int, fd *FieldDesc) ([]Record, int, error) {
	data, next, err := readLengthDelimited(buf, end, pos)
	if err != nil {
		return nil, 0, err
	}
	inner := buf[next-len(data) : next]
	var records []Record
	p := 0
	for p < len(inner) {
		rec := Record{Offset: next - len(data) + p, Field: fd.Number, Name: fd.Name, Type: string(fd.Type)}
		switch fd.Type {
		case TypeUint64:
			v, np, err := readVarint(inner, p)
			if err != nil {
				return nil, 0, err
			}
			rec.Value = v
			p = np
		case TypeSint64:
			v, np, err := readVarint(inner, p)
			if err != nil {
				return nil, 0, err
			}
			rec.Value = zigzagDecode(v)
			p = np
		case TypeFixed32:
			v, np, err := readFixed32(inner, p)
			if err != nil {
				return nil, 0, err
			}
			rec.Value = v
			p = np
		}
		records = append(records, rec)
	}
	return records, next, nil
}

// readLengthDelimited reads a varint length and returns the payload, which
// must lie fully within end.
func readLengthDelimited(buf []byte, end, pos int) ([]byte, int, error) {
	length, next, err := readVarint(buf[:end], pos)
	if err != nil {
		return nil, 0, err
	}
	if length > uint64(end-next) {
		return nil, 0, errAt(pos, "length %d exceeds remaining %d bytes of enclosing message", length, end-next)
	}
	return buf[next : next+int(length)], next + int(length), nil
}

// skipUnknown skips one unknown field value and returns the end position of
// the whole field (tag included via start for hex extraction).
func skipUnknown(buf []byte, pos, wire, tagStart int) (int, error) {
	switch wire {
	case WireVarint:
		_, next, err := readVarint(buf, pos)
		return next, err
	case WireFixed64:
		if pos+8 > len(buf) {
			return 0, errAt(pos, "truncated fixed64")
		}
		return pos + 8, nil
	case WireLengthDelimited:
		_, next, err := readLengthDelimited(buf, len(buf), pos)
		return next, err
	case WireFixed32:
		if pos+4 > len(buf) {
			return 0, errAt(pos, "truncated fixed32")
		}
		return pos + 4, nil
	}
	return 0, errAt(tagStart, "invalid wire type %d", wire)
}
