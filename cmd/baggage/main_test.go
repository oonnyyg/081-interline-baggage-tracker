package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"example.com/interline-baggage-tracker/internal/baggage"
)

func TestCLIEndToEnd(t *testing.T) {
	proc, err := baggage.OpenProcessor(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	input := strings.Join([]string{
		`{"op":"flight","id":"F1","from":"A","to":"B"}`,
		`{"op":"flight","id":"F2","from":"B","to":"C"}`,
		`{"op":"bag","label":"b1","segments":["F1","F2"]}`,
		`{"op":"scan","event":"e1","label":"b1","type":"load","flight":"F1","time":1}`,
		`not json`,
		`{"op":"scan","event":"e2","label":"b1","type":"unload","flight":"F1","time":2}`,
		`{"op":"query","label":"b1"}`,
		"",
	}, "\n")

	var out bytes.Buffer
	if err := run(proc, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("got %d output lines: %q", len(lines), out.String())
	}
	var bad map[string]any
	if err := json.Unmarshal([]byte(lines[4]), &bad); err != nil || bad["ok"] != false {
		t.Fatalf("invalid JSON line should yield error response, got %q", lines[4])
	}
	var query map[string]any
	if err := json.Unmarshal([]byte(lines[6]), &query); err != nil {
		t.Fatal(err)
	}
	status := query["status"].(map[string]any)
	if status["location"] != "B" || status["state"] != "ground" {
		t.Fatalf("unexpected status after processing: %v", status)
	}
}
