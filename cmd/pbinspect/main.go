package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	wireinspect "example.com/protobuf-wire-inspector"
)

type result struct {
	Line    int                  `json:"line"`
	OK      bool                 `json:"ok"`
	Records []wireinspect.Record `json:"records,omitempty"`
	Error   string               `json:"error,omitempty"`
}

type summary struct {
	Total  int `json:"total"`
	OK     int `json:"ok"`
	Failed int `json:"failed"`
}

func main() {
	schemaPath := flag.String("schema", "", "path to JSON field description (required)")
	hexInput := flag.String("hex", "", "single hex-encoded message to decode")
	batchPath := flag.String("batch", "", "path to JSONL file; each line is a hex string or {\"hex\": \"...\"}")
	flag.Parse()

	if *schemaPath == "" {
		fatal("-schema is required")
	}
	data, err := os.ReadFile(*schemaPath)
	if err != nil {
		fatal("read schema: %v", err)
	}
	schema, err := wireinspect.ParseSchema(data)
	if err != nil {
		fatal("%v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	switch {
	case *hexInput != "":
		emit(enc, decodeOne(1, *hexInput, schema))
	case *batchPath != "":
		f, err := os.Open(*batchPath)
		if err != nil {
			fatal("open batch: %v", err)
		}
		defer f.Close()
		runBatch(enc, f, schema)
	default:
		runBatch(enc, os.Stdin, schema)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pbinspect: "+format+"\n", args...)
	os.Exit(2)
}

func emit(enc *json.Encoder, r result) {
	if err := enc.Encode(r); err != nil {
		fatal("encode output: %v", err)
	}
}

func decodeOne(line int, hexText string, schema *wireinspect.Schema) result {
	r := result{Line: line}
	buf, err := hex.DecodeString(strings.TrimSpace(hexText))
	if err != nil {
		r.Error = "invalid hex: " + err.Error()
		return r
	}
	recs, err := wireinspect.Decode(buf, schema)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.OK = true
	r.Records = recs
	return r
}

func runBatch(enc *json.Encoder, in io.Reader, schema *wireinspect.Schema) {
	var sum summary
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		sum.Total++
		hexText := line
		if strings.HasPrefix(line, "{") {
			var obj struct {
				Hex string `json:"hex"`
			}
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				sum.Failed++
				emit(enc, result{Line: sum.Total, Error: "invalid JSONL entry: " + err.Error()})
				continue
			}
			hexText = obj.Hex
		} else {
			hexText = strings.Trim(line, `"`)
		}
		r := decodeOne(sum.Total, hexText, schema)
		if r.OK {
			sum.OK++
		} else {
			sum.Failed++
		}
		emit(enc, r)
	}
	if err := sc.Err(); err != nil {
		fatal("read input: %v", err)
	}
	if err := enc.Encode(struct {
		Summary summary `json:"summary"`
	}{sum}); err != nil {
		fatal("encode summary: %v", err)
	}
}
