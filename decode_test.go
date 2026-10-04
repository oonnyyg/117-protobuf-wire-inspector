package main

import (
	"encoding/hex"
	"testing"
)

const testSchema = `{
  "fields": [
    {"number": 1, "name": "id", "type": "uint64"},
    {"number": 2, "name": "delta", "type": "sint64"},
    {"number": 3, "name": "flags", "type": "fixed32"},
    {"number": 4, "name": "label", "type": "string"},
    {"number": 5, "name": "blob", "type": "bytes"},
    {"number": 6, "name": "samples", "type": "uint64", "repeated": true},
    {"number": 7, "name": "child", "type": "message", "message": {
      "fields": [
        {"number": 1, "name": "note", "type": "string"},
        {"number": 2, "name": "count", "type": "sint64"}
      ]
    }}
  ]
}`

func mustSchema(t *testing.T) *MessageDesc {
	t.Helper()
	desc, err := ParseMessageDesc([]byte(testSchema))
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return desc
}

func decodeHex(t *testing.T, desc *MessageDesc, hexPayload string) ([]Record, error) {
	t.Helper()
	payload, err := hex.DecodeString(hexPayload)
	if err != nil {
		t.Fatalf("bad test hex: %v", err)
	}
	return DecodeMessage(payload, 0, len(payload), desc)
}

func TestScalarsAndNegativeSint64(t *testing.T) {
	desc := mustSchema(t)
	// id=150, delta=-2 (zigzag 3), flags=0x78563412, label="hi", blob=0xdead
	recs, err := decodeHex(t, desc, "08960110031d12345678220268692a02dead")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(recs) != 5 {
		t.Fatalf("want 5 records, got %d", len(recs))
	}
	if recs[0].Name != "id" || recs[0].Value != uint64(150) {
		t.Errorf("id record: %+v", recs[0])
	}
	if recs[1].Name != "delta" || recs[1].Value != int64(-2) {
		t.Errorf("delta record: %+v", recs[1])
	}
	if recs[2].Name != "flags" || recs[2].Value != uint32(0x78563412) {
		t.Errorf("flags record: %+v", recs[2])
	}
	if recs[3].Name != "label" || recs[3].Value != "hi" {
		t.Errorf("label record: %+v", recs[3])
	}
	if recs[4].Name != "blob" || recs[4].Value != "dead" {
		t.Errorf("blob record: %+v", recs[4])
	}
}

func TestNestedMessageOffsets(t *testing.T) {
	desc := mustSchema(t)
	// child = {note:"a", count=-1}; outer prefix id=1
	recs, err := decodeHex(t, desc, "08013a050a01611001")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	child, ok := recs[1].Value.([]Record)
	if !ok || len(child) != 2 {
		t.Fatalf("child records: %+v", recs[1])
	}
	if child[0].Name != "note" || child[0].Value != "a" || child[0].Offset != 4 {
		t.Errorf("note record: %+v", child[0])
	}
	if child[1].Name != "count" || child[1].Value != int64(-1) || child[1].Offset != 7 {
		t.Errorf("count record: %+v", child[1])
	}
}
