package baggage

import "testing"

func mustHandle(t *testing.T, s *Store, req map[string]any) map[string]any {
	t.Helper()
	resp := s.Handle(req)
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("request %v rejected: %v", req, resp["error"])
	}
	return resp
}

func rejectHandle(t *testing.T, s *Store, req map[string]any) {
	t.Helper()
	resp := s.Handle(req)
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("request %v unexpectedly accepted: %v", req, resp)
	}
}

func seedFlights(t *testing.T, s *Store) {
	t.Helper()
	mustHandle(t, s, map[string]any{"op": "flight", "id": "F1", "from": "A", "to": "B"})
	mustHandle(t, s, map[string]any{"op": "flight", "id": "F2", "from": "B", "to": "C"})
	mustHandle(t, s, map[string]any{"op": "flight", "id": "G2", "from": "B", "to": "D"})
}

func TestFlightAndBagValidation(t *testing.T) {
	s := NewStore()
	rejectHandle(t, s, map[string]any{"op": "flight", "id": "X", "from": "A", "to": "A"})
	rejectHandle(t, s, map[string]any{"op": "flight", "id": "X"})
	seedFlights(t, s)
	rejectHandle(t, s, map[string]any{"op": "flight", "id": "F1", "from": "A", "to": "B"})

	rejectHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{}})
	rejectHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{"F1", "F1"}})
	rejectHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{"F2", "F1"}})
	rejectHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{"ZZ"}})
	mustHandle(t, s, map[string]any{"op": "bag", "label": "b1", "segments": []any{"F1", "F2"}})
	if got := s.bagsMust("b1").Location; got != "A" {
		t.Fatalf("initial location = %q, want A", got)
	}
}

func (s *Store) bagsMust(label string) *Bag {
	b, err := s.bag(label)
	if err != nil {
		panic(err)
	}
	return b
}
