package wireinspect

import (
	"encoding/json"
	"fmt"
)

// FieldType is a declared scalar or message type.
type FieldType string

const (
	TypeUint64  FieldType = "uint64"
	TypeSint64  FieldType = "sint64"
	TypeFixed32 FieldType = "fixed32"
	TypeString  FieldType = "string"
	TypeBytes   FieldType = "bytes"
	TypeMessage FieldType = "message"
)

// Field describes one declared field.
type Field struct {
	Number   uint32    `json:"number"`
	Name     string    `json:"name"`
	Type     FieldType `json:"type"`
	Repeated bool      `json:"repeated,omitempty"`
	Message  *Schema   `json:"message,omitempty"`
}

// Schema is an inline message description.
type Schema struct {
	Fields []*Field `json:"fields"`

	byNumber map[uint32]*Field
	byName   map[string]*Field
}

// ParseSchema decodes and validates a JSON schema document.
func ParseSchema(data []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("schema: invalid JSON: %w", err)
	}
	if err := s.validate("schema"); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Schema) validate(path string) error {
	s.byNumber = make(map[uint32]*Field)
	s.byName = make(map[string]*Field)
	for i, f := range s.Fields {
		if f == nil {
			return fmt.Errorf("%s: field %d is null", path, i)
		}
		fp := fmt.Sprintf("%s.field[%d]", path, i)
		if f.Number == 0 || f.Number > maxFieldNumber {
			return fmt.Errorf("%s: field number %d out of range 1..%d", fp, f.Number, maxFieldNumber)
		}
		if f.Name == "" {
			return fmt.Errorf("%s: name is required", fp)
		}
		if _, dup := s.byNumber[f.Number]; dup {
			return fmt.Errorf("%s: duplicate field number %d", fp, f.Number)
		}
		if _, dup := s.byName[f.Name]; dup {
			return fmt.Errorf("%s: duplicate field name %q", fp, f.Name)
		}
		switch f.Type {
		case TypeUint64, TypeSint64, TypeFixed32, TypeString, TypeBytes:
			if f.Message != nil {
				return fmt.Errorf("%s: message block only allowed for type message", fp)
			}
		case TypeMessage:
			if f.Message == nil {
				return fmt.Errorf("%s: message field requires inline message description", fp)
			}
			if err := f.Message.validate(fp + ".message"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unsupported type %q", fp, f.Type)
		}
		s.byNumber[f.Number] = f
		s.byName[f.Name] = f
	}
	return nil
}

func (s *Schema) fieldByNumber(n uint32) *Field {
	if s == nil {
		return nil
	}
	return s.byNumber[n]
}
