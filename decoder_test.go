package wireinspect

import (
	"encoding/hex"
	"strings"
	"testing"
)

func mustSchema(t *testing.T, js string) *Schema {
	t.Helper()
	s, err := ParseSchema([]byte(js))
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	return s
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func decodeOK(t *testing.T, js, hx string) []Record {
	t.Helper()
	recs, err := Decode(mustHex(t, hx), mustSchema(t, js))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return recs
}

func decodeErr(t *testing.T, js, hx string) *DecodeError {
	t.Helper()
	_, err := Decode(mustHex(t, hx), mustSchema(t, js))
	if err == nil {
		t.Fatalf("expected error, got none")
	}
	de, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	return de
}

const intsSchema = `{"fields":[
  {"number":1,"name":"id","type":"uint64"},
  {"number":2,"name":"delta","type":"sint64"},
  {"number":3,"name":"tag","type":"fixed32"},
  {"number":4,"name":"vals","type":"uint64","repeated":true},
  {"number":5,"name":"svals","type":"sint64","repeated":true}
]}`

func TestScalars(t *testing.T) {
	// id=150, delta=-1 (zigzag 1), tag=0x04030201
	recs := decodeOK(t, intsSchema, "08 96 01 10 01 1d 01 02 03 04")
	if len(recs) != 3 {
		t.Fatalf("got %d records", len(recs))
	}
	if recs[0].Value != uint64(150) || recs[0].Offset != 0 {
		t.Errorf("id record: %+v", recs[0])
	}
	if recs[1].Value != int64(-1) {
		t.Errorf("delta record: %+v", recs[1])
	}
	if recs[2].Value != uint32(0x04030201) {
		t.Errorf("tag record: %+v", recs[2])
	}
}

func TestSint64NegativePackedAndPlain(t *testing.T) {
	// svals: plain -1 (0x01), packed [1, -2] => 02 03
	recs := decodeOK(t, intsSchema, "28 01 2a 02 02 03")
	if len(recs) != 3 {
		t.Fatalf("got %d records", len(recs))
	}
	want := []int64{-1, 1, -2}
	for i, w := range want {
		if recs[i].Value != w {
			t.Errorf("rec %d: got %v want %d", i, recs[i].Value, w)
		}
	}
}

func TestPackedUint64(t *testing.T) {
	recs := decodeOK(t, intsSchema, "22 03 01 02 03")
	if len(recs) != 3 || recs[0].Value != uint64(1) || recs[2].Value != uint64(3) {
		t.Fatalf("packed: %+v", recs)
	}
}

func TestPackedNotRepeatedRejected(t *testing.T) {
	de := decodeErr(t, intsSchema, "0a 02 01 02") // id packed
	if de.Offset != 0 {
		t.Errorf("offset = %d, want 0", de.Offset)
	}
}

func TestWireTypeMismatch(t *testing.T) {
	de := decodeErr(t, intsSchema, "0d 01 02 03 04") // id as fixed32
	if de.Offset != 0 {
		t.Errorf("offset = %d, want 0", de.Offset)
	}
}

func TestInvalidFieldNumber(t *testing.T) {
	if de := decodeErr(t, intsSchema, "00"); de.Offset != 0 {
		t.Errorf("zero field: offset %d", de.Offset)
	}
	// field number 2^29 -> tag = (1<<29)<<3 | 0 = 0x400000000 varint: 80 80 80 80 10
	decodeErr(t, intsSchema, "80 80 80 80 10 01")
}

func TestInvalidWireType(t *testing.T) {
	decodeErr(t, intsSchema, "0f") // wire 7
	decodeErr(t, intsSchema, "0b") // group start
	decodeErr(t, intsSchema, "0c") // group end
}

func TestTruncated(t *testing.T) {
	decodeErr(t, intsSchema, "08")       // varint missing
	decodeErr(t, intsSchema, "08 96")    // varint truncated
	decodeErr(t, intsSchema, "1d 01 02") // fixed32 truncated
}

func TestVarintOverflow(t *testing.T) {
	decodeErr(t, intsSchema, "08 ff ff ff ff ff ff ff ff ff 7f")
}

func TestStringUTF8(t *testing.T) {
	js := `{"fields":[{"number":1,"name":"s","type":"string"}]}`
	recs := decodeOK(t, js, "0a 02 68 69")
	if recs[0].Value != "hi" {
		t.Errorf("string: %+v", recs[0])
	}
	decodeErr(t, js, "0a 01 ff") // invalid UTF-8
}

func TestBytesHex(t *testing.T) {
	js := `{"fields":[{"number":1,"name":"b","type":"bytes"}]}`
	recs := decodeOK(t, js, "0a 02 de ad")
	if recs[0].Value != "dead" {
		t.Errorf("bytes: %+v", recs[0])
	}
}

const nestedSchema = `{"fields":[
  {"number":1,"name":"outer","type":"message","message":{"fields":[
    {"number":1,"name":"inner_id","type":"uint64"},
    {"number":2,"name":"label","type":"string"}
  ]}}
]}`

func TestNestedMessage(t *testing.T) {
	// outer { inner_id: 7, label: "x" }
	recs := decodeOK(t, nestedSchema, "0a 06 08 07 12 02 01 78")
	if len(recs) != 1 {
		t.Fatalf("got %d records", len(recs))
	}
	sub, ok := recs[0].Value.([]Record)
	if !ok || len(sub) != 2 {
		t.Fatalf("nested value: %+v", recs[0].Value)
	}
	if sub[0].Path != "outer.inner_id" || sub[0].Value != uint64(7) {
		t.Errorf("nested rec: %+v", sub[0])
	}
}

func TestNestedTruncated(t *testing.T) {
	// declared length 5 but only 4 bytes of nested body present
	de := decodeErr(t, nestedSchema, "0a 05 08 07 12 02")
	if de.Offset != 1 {
		t.Errorf("offset = %d, want 1", de.Offset)
	}
	// nested body claims string longer than nested message
	de = decodeErr(t, nestedSchema, "0a 04 12 03 01 78")
	if de.Offset != 3 {
		t.Errorf("offset = %d, want 3", de.Offset)
	}
}

func TestUnknownFieldsSkipped(t *testing.T) {
	js := `{"fields":[{"number":2,"name":"id","type":"uint64"}]}`
	// unknown varint field 1, unknown bytes field 3, unknown fixed32 field 4, then id=9
	recs := decodeOK(t, js, "08 2a 1a 02 aa bb 25 01 02 03 04 10 09")
	if len(recs) != 4 {
		t.Fatalf("got %d records", len(recs))
	}
	if !recs[0].Unknown || recs[0].Number != 1 || recs[0].Hex != "2a" {
		t.Errorf("unknown varint: %+v", recs[0])
	}
	if recs[1].Hex != "02aabb" {
		t.Errorf("unknown bytes: %+v", recs[1])
	}
	if recs[2].Hex != "01020304" {
		t.Errorf("unknown fixed32: %+v", recs[2])
	}
	if recs[3].Name != "id" || recs[3].Value != uint64(9) {
		t.Errorf("known after unknown: %+v", recs[3])
	}
}

func TestDuplicateSchemaRejected(t *testing.T) {
	if _, err := ParseSchema([]byte(`{"fields":[
		{"number":1,"name":"a","type":"uint64"},
		{"number":1,"name":"b","type":"uint64"}]}`)); err == nil {
		t.Error("duplicate number accepted")
	}
	if _, err := ParseSchema([]byte(`{"fields":[
		{"number":1,"name":"a","type":"uint64"},
		{"number":2,"name":"a","type":"uint64"}]}`)); err == nil {
		t.Error("duplicate name accepted")
	}
}
