package main

import (
	"encoding/json"
	"fmt"
)

// FieldType names a supported scalar or message type.
type FieldType string

const (
	TypeUint64  FieldType = "uint64"
	TypeSint64  FieldType = "sint64"
	TypeFixed32 FieldType = "fixed32"
	TypeString  FieldType = "string"
	TypeBytes   FieldType = "bytes"
	TypeMessage FieldType = "message"
)

// FieldDesc describes one declared field.
type FieldDesc struct {
	Number   int
	Name     string
	Type     FieldType
	Repeated bool
	Message  *MessageDesc // inline nested message, when Type == TypeMessage
}

// MessageDesc is a set of field descriptors keyed by number and name.
type MessageDesc struct {
	ByNumber map[int]*FieldDesc
	ByName   map[string]*FieldDesc
}

// fieldJSON is the on-disk JSON form of a field descriptor.
type fieldJSON struct {
	Number   int             `json:"number"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Repeated bool            `json:"repeated"`
	Message  json.RawMessage `json:"message"`
}

type messageJSON struct {
	Fields []fieldJSON `json:"fields"`
}

// ParseMessageDesc parses and validates a JSON message descriptor.
func ParseMessageDesc(data []byte) (*MessageDesc, error) {
	var raw messageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid message descriptor JSON: %w", err)
	}
	return buildMessageDesc(&raw, "message")
}

func buildMessageDesc(raw *messageJSON, path string) (*MessageDesc, error) {
	desc := &MessageDesc{
		ByNumber: make(map[int]*FieldDesc),
		ByName:   make(map[string]*FieldDesc),
	}
	for i, f := range raw.Fields {
		where := fmt.Sprintf("%s field %d", path, i)
		if f.Number <= 0 || f.Number > maxFieldNumber {
			return nil, fmt.Errorf("%s: number %d out of range 1..%d", where, f.Number, maxFieldNumber)
		}
		if f.Name == "" {
			return nil, fmt.Errorf("%s: name is required", where)
		}
		if _, dup := desc.ByNumber[f.Number]; dup {
			return nil, fmt.Errorf("%s: duplicate field number %d", where, f.Number)
		}
		if _, dup := desc.ByName[f.Name]; dup {
			return nil, fmt.Errorf("%s: duplicate field name %q", where, f.Name)
		}
		fd := &FieldDesc{
			Number:   f.Number,
			Name:     f.Name,
			Type:     FieldType(f.Type),
			Repeated: f.Repeated,
		}
		switch fd.Type {
		case TypeUint64, TypeSint64, TypeFixed32, TypeString, TypeBytes:
			if len(f.Message) != 0 {
				return nil, fmt.Errorf("%s: %q is not a message type but has a nested message", where, f.Name)
			}
		case TypeMessage:
			if len(f.Message) == 0 {
				return nil, fmt.Errorf("%s: message field %q requires an inline \"message\" descriptor", where, f.Name)
			}
			var nested messageJSON
			if err := json.Unmarshal(f.Message, &nested); err != nil {
				return nil, fmt.Errorf("%s: invalid nested descriptor for %q: %w", where, f.Name, err)
			}
			sub, err := buildMessageDesc(&nested, path+"."+f.Name)
			if err != nil {
				return nil, err
			}
			fd.Message = sub
		default:
			return nil, fmt.Errorf("%s: unsupported type %q", where, f.Type)
		}
		desc.ByNumber[fd.Number] = fd
		desc.ByName[fd.Name] = fd
	}
	return desc, nil
}

// wireType returns the canonical wire type for a non-packed field.
func (f *FieldDesc) wireType() int {
	switch f.Type {
	case TypeUint64, TypeSint64:
		return WireVarint
	case TypeFixed32:
		return WireFixed32
	default: // string, bytes, message
		return WireLengthDelimited
	}
}

// packable reports whether the field may use packed length-delimited encoding.
func (f *FieldDesc) packable() bool {
	return f.Type == TypeUint64 || f.Type == TypeSint64 || f.Type == TypeFixed32
}
