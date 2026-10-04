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
)

type inputLine struct {
	Hex string `json:"hex"`
}

type outputLine struct {
	Line    int      `json:"line"`
	OK      bool     `json:"ok"`
	Records []Record `json:"records,omitempty"`
	Error   string   `json:"error,omitempty"`
	Offset  int      `json:"offset,omitempty"`
}

func main() {
	schemaPath := flag.String("schema", "", "path to JSON field descriptor file (required)")
	inputPath := flag.String("input", "", "path to JSONL input file (default: stdin)")
	flag.Parse()
	if *schemaPath == "" {
		fmt.Fprintln(os.Stderr, "usage: wireinspect -schema schema.json [-input messages.jsonl]")
		os.Exit(2)
	}
	if err := run(*schemaPath, *inputPath, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
func run(schemaPath, inputPath string, stdout, stderr io.Writer) error {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("reading schema: %w", err)
	}
	desc, err := ParseMessageDesc(schemaData)
	if err != nil {
		return err
	}

	in := io.Reader(os.Stdin)
	if inputPath != "" {
		f, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("opening input: %w", err)
		}
		defer f.Close()
		in = f
	}

	enc := json.NewEncoder(stdout)
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	total, ok, failed := 0, 0, 0
	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		total++
		out := processLine(line, lineNo, desc)
		if out.OK {
			ok++
		} else {
			failed++
		}
		if err := enc.Encode(out); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	fmt.Fprintf(stderr, "summary: total=%d ok=%d failed=%d\n", total, ok, failed)
	return nil
}

func processLine(line string, lineNo int, desc *MessageDesc) outputLine {
	out := outputLine{Line: lineNo}
	var input inputLine
	if err := json.Unmarshal([]byte(line), &input); err != nil {
		out.Error = "invalid JSONL input: " + err.Error()
		return out
	}
	payload, err := hex.DecodeString(strings.TrimSpace(input.Hex))
	if err != nil {
		out.Error = "invalid hex payload: " + err.Error()
		return out
	}
	records, err := DecodeMessage(payload, 0, len(payload), desc)
	if err != nil {
		out.Error = err.Error()
		if de, ok := err.(*DecodeError); ok {
			out.Offset = de.Offset
		}
		return out
	}
	out.OK = true
	out.Records = records
	return out
}
