package baggage

import "testing"

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewState()
	mustRegister(t, s, Flight{Number: "F1", From: "A", To: "B"})
	if err := s.RegisterBag("B1", []Segment{{Flight: "F1", From: "A", To: "B"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyScan(ScanEvent{ID: 1, Type: "load", Label: "B1", Time: 5, Flight: "F1"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Flights["F1"]; !ok {
		t.Fatal("flight not persisted")
	}
	bag := loaded.Bags["B1"]
	if bag == nil || bag.Loaded != "F1" || bag.LastTime == nil || *bag.LastTime != 5 {
		t.Fatalf("bag state not restored: %+v", bag)
	}
	cached, ok := loaded.Events[1]
	if !ok || cached.Hash == "" || len(cached.Body) == 0 {
		t.Fatal("idempotency cache not persisted")
	}

	empty, err := Load(t.TempDir())
	if err != nil || empty.Flights == nil {
		t.Fatalf("missing state file should yield empty state: %v err=%v", empty, err)
	}
}
