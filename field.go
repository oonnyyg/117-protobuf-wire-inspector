package wireinspect

import (
	"encoding/binary"
	"encoding/hex"
	"unicode/utf8"
)

// expectedWire returns the canonical wire type for a declared type.
func expectedWire(t FieldType) int {
	switch t {
	case TypeUint64, TypeSint64:
		return wireVarint
	case TypeFixed32:
		return wireFixed32
	default: // string, bytes, message
		return wireBytes
	}
}

func isPackable(t FieldType) bool {
	return t == TypeUint64 || t == TypeSint64 || t == TypeFixed32
}

func (d *decoder) knownField(tagOff, pos int, f *Field, wire, end int, path string) ([]Record, int, error) {
	exp := expectedWire(f.Type)
	// Packed encoding: length-delimited payload for repeated integers.
	if wire == wireBytes && exp != wireBytes && isPackable(f.Type) {
		if !f.Repeated {
			return nil, 0, errf(tagOff, "field %q: packed encoding requires repeated", f.Name)
		}
		return d.packedField(tagOff, pos, f, end, path)
	}
	if wire != exp {
		return nil, 0, errf(tagOff, "field %q: wire type %d does not match declared type %s (want wire %d)",
			f.Name, wire, f.Type, exp)
	}
	rec := Record{
		Path: joinPath(path, f.Name), Number: f.Number,
		Name: f.Name, Type: string(f.Type), Offset: tagOff,
	}
	switch f.Type {
	case TypeUint64:
		v, next, err := d.varint(pos)
		if err != nil {
			return nil, 0, err
		}
		rec.Value = v
		return []Record{rec}, next, nil
	case TypeSint64:
		v, next, err := d.varint(pos)
		if err != nil {
			return nil, 0, err
		}
		rec.Value = int64(v>>1) ^ -int64(v&1)
		return []Record{rec}, next, nil
	case TypeFixed32:
		if pos+4 > end {
			return nil, 0, errf(pos, "truncated fixed32")
		}
		rec.Value = binary.LittleEndian.Uint32(d.buf[pos : pos+4])
		return []Record{rec}, pos + 4, nil
	case TypeString, TypeBytes, TypeMessage:
		l, next, err := d.varint(pos)
		if err != nil {
			return nil, 0, err
		}
		if l > uint64(end-next) {
			return nil, 0, errf(pos, "length %d exceeds remaining %d bytes", l, end-next)
		}
		body := d.buf[next : next+int(l)]
		switch f.Type {
		case TypeString:
			if !utf8.Valid(body) {
				return nil, 0, errf(next, "field %q: invalid UTF-8 string", f.Name)
			}
			rec.Value = string(body)
		case TypeBytes:
			rec.Value = hex.EncodeToString(body)
		case TypeMessage:
			sub, err := d.message(next, next+int(l), f.Message, rec.Path)
			if err != nil {
				return nil, 0, err
			}
			rec.Value = sub
		}
		return []Record{rec}, next + int(l), nil
	}
	return nil, 0, errf(tagOff, "unsupported type %s", f.Type)
}

// packedField decodes a length-delimited run of packed integers,
// emitting one record per element to preserve order.
func (d *decoder) packedField(tagOff, pos int, f *Field, end int, path string) ([]Record, int, error) {
	l, next, err := d.varint(pos)
	if err != nil {
		return nil, 0, err
	}
	if l > uint64(end-next) {
		return nil, 0, errf(pos, "length %d exceeds remaining %d bytes", l, end-next)
	}
	lo, hi := next, next+int(l)
	var recs []Record
	p := lo
	for p < hi {
		rec := Record{
			Path: joinPath(path, f.Name), Number: f.Number,
			Name: f.Name, Type: string(f.Type), Offset: tagOff,
		}
		switch f.Type {
		case TypeUint64, TypeSint64:
			v, np, err := d.varint(p)
			if err != nil {
				return nil, 0, err
			}
			if np > hi {
				return nil, 0, errf(p, "packed element overruns length-delimited block")
			}
			if f.Type == TypeSint64 {
				rec.Value = int64(v>>1) ^ -int64(v&1)
			} else {
				rec.Value = v
			}
			p = np
		case TypeFixed32:
			if p+4 > hi {
				return nil, 0, errf(p, "packed fixed32 overruns length-delimited block")
			}
			rec.Value = binary.LittleEndian.Uint32(d.buf[p : p+4])
			p += 4
		}
		recs = append(recs, rec)
	}
	return recs, hi, nil
}
