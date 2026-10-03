package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI executes the compiled CLI as a subprocess so stdin/stdout line
// framing and state persistence are exercised end to end.
func runCLI(t *testing.T, dir string, input string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "baggage.exe")
	build := exec.Command("go", "build", "-o", exe, ".")
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(exe, "--state", dir)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	return string(out)
}

func decodeLines(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestCLIFullSession(t *testing.T) {
	input := strings.Join([]string{
		`{"op":"register-flight","number":"F1","from":"A","to":"B"}`,
		`{"op":"register-flight","number":"F2","from":"B","to":"C"}`,
		`{"op":"register-flight","number":"F2","from":"B","to":"X"}`,
		`{"op":"register-bag","label":"B1","segments":[{"flight":"F1","from":"A","to":"B"},{"flight":"F2","from":"B","to":"C"}]}`,
		`{"op":"scan","id":1,"type":"load","label":"B1","time":1,"flight":"F2"}`,
		`{"op":"scan","id":1,"type":"load","label":"B1","time":1,"flight":"F2"}`,
		`{"op":"scan","id":1,"type":"load","label":"B1","time":2,"flight":"F2"}`,
		`{"op":"scan","id":2,"type":"load","label":"B1","time":2,"flight":"F1"}`,
		`{"op":"scan","id":2,"type":"load","label":"B1","time":2,"flight":"F1"}`,
		`{"op":"scan","id":3,"type":"remove","label":"B1","time":3,"flight":"F1"}`,
		`{"op":"scan","id":4,"type":"load","label":"B1","time":4,"flight":"F1"}`,
		`{"op":"scan","id":5,"type":"unload","label":"B1","time":5,"flight":"F1"}`,
		`{"op":"scan","id":6,"type":"load","label":"B1","time":6,"flight":"F2"}`,
		`{"op":"scan","id":7,"type":"unload","label":"B1","time":7,"flight":"F2"}`,
		`{"op":"scan","id":8,"type":"deliver","label":"B1","time":8}`,
		`{"op":"query","label":"B1"}`,
		`{"op":"query","label":"MISSING"}`,
		`{"op":"bogus"}`,
		`not json`,
	}, "\n")

	dir := t.TempDir()
	lines := decodeLines(t, runCLI(t, dir, input))

	okFlags := []bool{true, true, false, true, false, false, false, true, true, true, true, true, true, true, true, true, false, false, false}
	if len(lines) != len(okFlags) {
		t.Fatalf("got %d lines, want %d: %v", len(lines), len(okFlags), lines)
	}
	for i, wantOK := range okFlags {
		if got, _ := lines[i]["ok"].(bool); got != wantOK {
			t.Fatalf("line %d ok=%v want %v: %v", i, got, wantOK, lines[i])
		}
	}

	// Index 8 is the identical duplicate of the successful id=2 load.
	result := lines[8]["result"].(map[string]any)
	if replayed, _ := result["replayed"].(bool); !replayed {
		t.Fatalf("duplicate event should be replayed: %v", result)
	}
	// Failed wrong-order load must not consume id 1; later valid load still works.
	queryResult := lines[15]["result"].(map[string]any)
	if delivered, _ := queryResult["delivered"].(bool); !delivered {
		t.Fatalf("bag should end delivered: %v", queryResult)
	}
	scans := queryResult["scans"].([]any)
	if len(scans) != 7 {
		t.Fatalf("want 7 scans (failed/replayed rows excluded), got %d", len(scans))
	}

	// Reopen with a new process: state continues and duplicate stays rejected.
	reopen := strings.Join([]string{
		`{"op":"scan","id":2,"type":"load","label":"B1","time":99,"flight":"F2"}`,
		`{"op":"query","label":"B1"}`,
	}, "\n")
	reopened := decodeLines(t, runCLI(t, dir, reopen))
	if ok, _ := reopened[0]["ok"].(bool); ok {
		t.Fatalf("id collision must survive restart: %v", reopened[0])
	}
	if ok, _ := reopened[1]["ok"].(bool); !ok {
		t.Fatalf("query after restart failed: %v", reopened[1])
	}
}
