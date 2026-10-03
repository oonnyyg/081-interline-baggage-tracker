package baggage

import (
	"path/filepath"
	"testing"
)

func TestJournalReplayAndErrorIsolation(t *testing.T) {
	dir := t.TempDir()

	p, err := OpenProcessor(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	submit := func(req map[string]any) map[string]any {
		t.Helper()
		resp := p.Process(req)
		if ok, _ := resp["ok"].(bool); !ok {
			t.Fatalf("request %v rejected: %v", req, resp["error"])
		}
		return resp
	}
	submit(map[string]any{"op": "flight", "id": "F1", "from": "A", "to": "B"})
	submit(map[string]any{"op": "bag", "label": "b1", "segments": []any{"F1"}})
	// Invalid line: wrong next flight does not exist; must not be persisted.
	resp := p.Process(map[string]any{"op": "scan", "event": "x", "label": "b1", "type": "load", "flight": "NOPE", "time": 1})
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatal("invalid scan accepted")
	}
	submit(map[string]any{"op": "scan", "event": "e1", "label": "b1", "type": "load", "flight": "F1", "time": 1})
	// Queries are never journaled.
	submit(map[string]any{"op": "query", "label": "b1"})
	p2, err := OpenProcessor(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	q := p2.Process(map[string]any{"op": "query", "label": "b1"})
	status := q["status"].(map[string]any)
	if status["state"] != "loaded" || status["flight"] != "F1" {
		t.Fatalf("replayed state wrong: %v", status)
	}
	// Event ids survive restart: exact duplicate returns prior result without
	// a new scan.
	dup := p2.Process(map[string]any{"op": "scan", "event": "e1", "label": "b1", "type": "load", "flight": "F1", "time": 1})
	if ok, _ := dup["ok"].(bool); !ok {
		t.Fatalf("duplicate after restart rejected: %v", dup)
	}
	conflict := p2.Process(map[string]any{"op": "scan", "event": "e1", "label": "b1", "type": "unload", "flight": "F1", "time": 2})
	if ok, _ := conflict["ok"].(bool); ok {
		t.Fatal("conflicting event id accepted")
	}
}
