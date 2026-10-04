package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchJSONL(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(testSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "in.jsonl")
	input := strings.Join([]string{
		`{"hex":"089601"}`,         // ok
		`{"hex":"0880"}`,           // truncated varint
		`{"hex":"zz"}`,             // bad hex
		`not json`,                 // bad json
		`{"hex":"3a050a01611001"}`, // ok nested
	}, "\n")
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := run(schemaPath, inputPath, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr.String(), "total=5 ok=2 failed=3") {
		t.Errorf("summary: %q", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("want 5 output lines, got %d", len(lines))
	}
	var outs []outputLine
	for _, l := range lines {
		var o outputLine
		if err := json.Unmarshal([]byte(l), &o); err != nil {
			t.Fatalf("output not JSON: %v", err)
		}
		outs = append(outs, o)
	}
	if !outs[0].OK || outs[0].Records[0].Value != float64(150) {
		t.Errorf("line 1: %+v", outs[0])
	}
	if outs[1].OK || outs[1].Offset != 1 || !strings.Contains(outs[1].Error, "truncated varint") {
		t.Errorf("line 2: %+v", outs[1])
	}
	if outs[2].OK || !strings.Contains(outs[2].Error, "invalid hex") {
		t.Errorf("line 3: %+v", outs[2])
	}
	if outs[3].OK || !strings.Contains(outs[3].Error, "invalid JSONL") {
		t.Errorf("line 4: %+v", outs[3])
	}
	if !outs[4].OK || len(outs[4].Records) != 1 {
		t.Errorf("line 5: %+v", outs[4])
	}
}
