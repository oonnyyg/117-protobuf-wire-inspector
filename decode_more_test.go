package main

import (
	"strings"
	"testing"
)

func TestMixedPackedAndSingular(t *testing.T) {
	desc := mustSchema(t)
	// samples=1 (varint), samples packed [2,3], samples=4 (varint)
	recs, err := decodeHex(t, desc, "3001320202033004")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(recs) != 4 {
		t.Fatalf("want 4 records, got %d", len(recs))
	}
	want := []uint64{1, 2, 3, 4}
	for i, w := range want {
		if recs[i].Name != "samples" || recs[i].Value != w {
			t.Errorf("record %d: %+v", i, recs[i])
		}
	}
	if recs[1].Offset != 4 || recs[2].Offset != 5 {
		t.Errorf("packed offsets: %d, %d", recs[1].Offset, recs[2].Offset)
	}
}

func TestUnknownFieldsSkipped(t *testing.T) {
	desc := mustSchema(t)
	// unknown field 9 varint, unknown field 10 length-delimited, then id=7
	recs, err := decodeHex(t, desc, "4807520268690807")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("want 3 records, got %d", len(recs))
	}
	if !recs[0].Unknown || recs[0].Field != 9 || recs[0].Hex != "4807" {
		t.Errorf("unknown varint: %+v", recs[0])
	}
	if !recs[1].Unknown || recs[1].Field != 10 || recs[1].Hex != "52026869" {
		t.Errorf("unknown bytes: %+v", recs[1])
	}
	if recs[2].Name != "id" || recs[2].Value != uint64(7) {
		t.Errorf("id after unknowns: %+v", recs[2])
	}
}

func TestDecodeErrors(t *testing.T) {
	desc := mustSchema(t)
	cases := []struct {
		name    string
		hex     string
		wantErr string
		offset  int
	}{
		{"zero field number", "0001", "field number 0", 0},
		{"invalid wire type", "0b", "invalid wire type 3", 0},
		{"truncated varint", "0880", "truncated varint", 1},
		{"varint overflow", "08ffffffffffffffffff7f", "overflows 64 bits", 1},
		{"truncated fixed32", "1d1234", "truncated fixed32", 1},
		{"wire type mismatch", "0d01020304", "does not match declared type", 0},
		{"length exceeds message", "22056869", "exceeds remaining", 1},
		{"invalid utf8", "2201ff", "invalid UTF-8", 1},
		{"nested truncated", "3a040a03616", "truncated varint", 0}, // placeholder replaced below
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "nested truncated" {
				return // covered by dedicated test
			}
			_, err := decodeHex(t, desc, tc.hex)
			if err == nil {
				t.Fatalf("want error containing %q", tc.wantErr)
			}
			de, ok := err.(*DecodeError)
			if !ok {
				t.Fatalf("want *DecodeError, got %T", err)
			}
			if !strings.Contains(de.Msg, tc.wantErr) {
				t.Errorf("error %q does not contain %q", de.Msg, tc.wantErr)
			}
			if de.Offset != tc.offset {
				t.Errorf("offset: want %d, got %d", tc.offset, de.Offset)
			}
		})
	}
}

func TestNestedTruncationOffset(t *testing.T) {
	desc := mustSchema(t)
	// child declares length 4 but its inner string length (3) overruns the
	// nested message boundary; offset is relative to the whole packet.
	_, err := decodeHex(t, desc, "08013a040a036162")
	if err == nil {
		t.Fatal("want error")
	}
	de := err.(*DecodeError)
	if !strings.Contains(de.Msg, "exceeds remaining") {
		t.Errorf("unexpected error: %v", de)
	}
	if de.Offset != 5 {
		t.Errorf("offset: want 5, got %d", de.Offset)
	}
}

func TestSchemaValidation(t *testing.T) {
	cases := []struct {
		name    string
		schema  string
		wantErr string
	}{
		{"duplicate number", `{"fields":[{"number":1,"name":"a","type":"uint64"},{"number":1,"name":"b","type":"uint64"}]}`, "duplicate field number"},
		{"duplicate name", `{"fields":[{"number":1,"name":"a","type":"uint64"},{"number":2,"name":"a","type":"uint64"}]}`, "duplicate field name"},
		{"bad number", `{"fields":[{"number":0,"name":"a","type":"uint64"}]}`, "out of range"},
		{"bad type", `{"fields":[{"number":1,"name":"a","type":"float"}]}`, "unsupported type"},
		{"message without body", `{"fields":[{"number":1,"name":"a","type":"message"}]}`, "requires an inline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseMessageDesc([]byte(tc.schema))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
