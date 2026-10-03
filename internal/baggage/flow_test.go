package baggage

import (
	"reflect"
	"testing"
)

func loadUnload(t *testing.T, s *Store, label, flight string, loadTime, unloadTime int64, loadEvent, unloadEvent string) {
	t.Helper()
	mustHandle(t, s, map[string]any{"op": "scan", "event": loadEvent, "label": label, "type": "load", "flight": flight, "time": loadTime})
	mustHandle(t, s, map[string]any{"op": "scan", "event": unloadEvent, "label": label, "type": "unload", "flight": flight, "time": unloadTime})
}

func TestFullJourneyAndDelivery(t *testing.T) {
	s := NewStore()
	seedFlights(t, s)
	mustHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{"F1", "F2"}})

	// Cannot load the second segment first, nor from the wrong airport.
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "x", "label": "b1", "type": "load", "flight": "F2", "time": 1})

	loadUnload(t, s, "b1", "F1", 10, 20, "e1", "e2")
	// Times must be strictly increasing for the same bag.
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "x", "label": "b1", "type": "load", "flight": "F2", "time": 20})
	b := s.bagsMust("b1")
	if b.Location != "B" || len(b.Completed) != 1 || b.Remaining[0] != "F2" {
		t.Fatalf("after first leg state wrong: %+v", b)
	}

	// Completed segment cannot be loaded again.
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "x", "label": "b1", "type": "load", "flight": "F1", "time": 21})

	loadUnload(t, s, "b1", "F2", 30, 40, "e3", "e4")
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "x", "label": "b1", "type": "delivered", "flight": "F2", "time": 41})
	mustHandle(t, s, map[string]any{"op": "scan", "event": "e5", "label": "b1", "type": "delivered", "time": 50})
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "e6", "label": "b1", "type": "load", "flight": "F1", "time": 60})
	rejectHandle(t, s, map[string]any{"op": "rebook", "label": "b1", "segments": []any{"F1"}})

	q := mustHandle(t, s, map[string]any{"op": "query", "label": "b1"})
	status := q["status"].(map[string]any)
	if status["state"] != "delivered" || status["location"] != "C" {
		t.Fatalf("final status wrong: %v", status)
	}
}

func TestRemoveAndRebook(t *testing.T) {
	s := NewStore()
	seedFlights(t, s)
	mustHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{"F1", "F2"}})
	loadUnload(t, s, "b1", "F1", 1, 2, "e1", "e2")

	// Load next segment, then pull it back off at the departure airport.
	mustHandle(t, s, map[string]any{"op": "scan", "event": "e3", "label": "b1", "type": "load", "flight": "F2", "time": 3})
	// Rebooking while loaded is forbidden.
	rejectHandle(t, s, map[string]any{"op": "rebook", "label": "b1", "segments": []any{"G2"}})
	// Cannot unload F2 while still on the ground notionally; remove first.
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "x", "label": "b1", "type": "remove", "flight": "F1", "time": 4})
	mustHandle(t, s, map[string]any{"op": "scan", "event": "e4", "label": "b1", "type": "remove", "flight": "F2", "time": 5})

	b := s.bagsMust("b1")
	if b.Location != "B" || len(b.Completed) != 1 || b.Remaining[0] != "F2" {
		t.Fatalf("after remove state wrong: %+v", b)
	}

	// New plan must start at B, stay non-empty and avoid completed flights.
	rejectHandle(t, s, map[string]any{"op": "rebook", "label": "b1", "segments": []any{}})
	rejectHandle(t, s, map[string]any{"op": "rebook", "label": "b1", "segments": []any{"F1"}})
	rejectHandle(t, s, map[string]any{"op": "rebook", "label": "b1", "segments": []any{"F2", "F2"}})
	mustHandle(t, s, map[string]any{"op": "rebook", "label": "b1", "segments": []any{"G2"}})

	b = s.bagsMust("b1")
	if b.Remaining[0] != "G2" || len(b.Completed) != 1 || b.Segments[0] != "F1" {
		t.Fatalf("rebooked state wrong: %+v", b)
	}
	// The removed segment F2 may now be flown? No: plan is G2; loading F2 fails.
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "x", "label": "b1", "type": "load", "flight": "F2", "time": 6})
	mustHandle(t, s, map[string]any{"op": "scan", "event": "e6", "label": "b1", "type": "load", "flight": "G2", "time": 7})
}

func TestEventDeduplication(t *testing.T) {
	s := NewStore()
	seedFlights(t, s)
	mustHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{"F1"}})

	req := map[string]any{"op": "scan", "event": "dup", "label": "b1", "type": "load", "flight": "F1", "time": 1}
	first := mustHandle(t, s, req)
	second := mustHandle(t, s, req)
	if !reflect.DeepEqual(first["status"], second["status"]) {
		t.Fatalf("duplicate submission returned different result")
	}
	if len(s.bagsMust("b1").Scans) != 1 {
		t.Fatalf("duplicate scan recorded twice")
	}
	// Same event id with different content is rejected and does not consume it.
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "dup", "label": "b1", "type": "unload", "flight": "F1", "time": 1})
	// Original still returns the stored result.
	mustHandle(t, s, req)

	// A failed scan does not consume its event id or advance time.
	rejectHandle(t, s, map[string]any{"op": "scan", "event": "bad", "label": "b1", "type": "unload", "flight": "F1", "time": 0})
	mustHandle(t, s, map[string]any{"op": "scan", "event": "bad", "label": "b1", "type": "unload", "flight": "F1", "time": 2})
}
