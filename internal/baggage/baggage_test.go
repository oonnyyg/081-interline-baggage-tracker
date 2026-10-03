package baggage

import "testing"

func mustRegister(t *testing.T, s *State, flights ...Flight) {
	t.Helper()
	for _, f := range flights {
		if err := s.RegisterFlight(f); err != nil {
			t.Fatalf("register flight %s: %v", f.Number, err)
		}
	}
}

func TestFullInterlineJourney(t *testing.T) {
	s := NewState()
	mustRegister(t, s,
		Flight{Number: "AA1", From: "PEK", To: "SHA"},
		Flight{Number: "BB2", From: "SHA", To: "CAN"},
		Flight{Number: "CC3", From: "CAN", To: "CTU"},
	)
	plan := []Segment{
		{Flight: "AA1", From: "PEK", To: "SHA"},
		{Flight: "BB2", From: "SHA", To: "CAN"},
		{Flight: "CC3", From: "CAN", To: "CTU"},
	}
	if err := s.RegisterBag("B1", plan); err != nil {
		t.Fatalf("register bag: %v", err)
	}
	view, _ := s.Query("B1")
	if view.Location != "PEK" || len(view.Completed) != 0 || len(view.Remaining) != 3 {
		t.Fatalf("unexpected initial view: %+v", view)
	}

	steps := []ScanEvent{
		{ID: 1, Type: "load", Label: "B1", Time: 1, Flight: "AA1"},
		{ID: 2, Type: "unload", Label: "B1", Time: 2, Flight: "AA1"},
		{ID: 3, Type: "load", Label: "B1", Time: 3, Flight: "BB2"},
		{ID: 4, Type: "unload", Label: "B1", Time: 4, Flight: "BB2"},
		{ID: 5, Type: "load", Label: "B1", Time: 5, Flight: "CC3"},
		{ID: 6, Type: "unload", Label: "B1", Time: 6, Flight: "CC3"},
	}
	for _, ev := range steps {
		if _, err := s.ApplyScan(ev); err != nil {
			t.Fatalf("apply %+v: %v", ev, err)
		}
	}
	if r, err := s.ApplyScan(ScanEvent{ID: 7, Type: "deliver", Label: "B1", Time: 7}); err != nil || !r.Delivered {
		t.Fatalf("deliver: result=%+v err=%v", r, err)
	}

	if _, err := s.ApplyScan(ScanEvent{ID: 8, Type: "load", Label: "B1", Time: 8, Flight: "AA1"}); err == nil {
		t.Fatal("expected scan on delivered bag to fail")
	}
}

func TestRegistrationValidation(t *testing.T) {
	s := NewState()
	if err := s.RegisterFlight(Flight{Number: "F1", From: "X", To: "X"}); err == nil {
		t.Fatal("expected same-airport flight rejection")
	}
	mustRegister(t, s,
		Flight{Number: "F1", From: "A", To: "B"},
		Flight{Number: "F2", From: "B", To: "C"},
		Flight{Number: "F3", From: "C", To: "D"},
	)
	cases := [][]Segment{
		nil,
		{{Flight: "F1", From: "A", To: "B"}, {Flight: "F1", From: "A", To: "B"}},
		{{Flight: "F1", From: "A", To: "B"}, {Flight: "F3", From: "C", To: "D"}},
		{{Flight: "F1", From: "A", To: "C"}},
		{{Flight: "F404", From: "A", To: "B"}},
	}
	for i, plan := range cases {
		if err := s.RegisterBag("BAD", plan); err == nil {
			t.Fatalf("case %d: expected rejection", i)
		}
	}
	if _, ok := s.Bags["BAD"]; ok {
		t.Fatal("failed registration must not create a bag")
	}
}

func TestWrongOrderAndTimestamps(t *testing.T) {
	s := NewState()
	mustRegister(t, s,
		Flight{Number: "F1", From: "A", To: "B"},
		Flight{Number: "F2", From: "B", To: "C"},
	)
	if err := s.RegisterBag("B1", []Segment{{Flight: "F1", From: "A", To: "B"}, {Flight: "F2", From: "B", To: "C"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ApplyScan(ScanEvent{ID: 1, Type: "load", Label: "B1", Time: 10, Flight: "F2"}); err == nil {
		t.Fatal("loading non-next flight must fail")
	}
	if _, err := s.ApplyScan(ScanEvent{ID: 2, Type: "unload", Label: "B1", Time: 11, Flight: "F1"}); err == nil {
		t.Fatal("unloading a ground bag must fail")
	}
	if _, err := s.ApplyScan(ScanEvent{ID: 3, Type: "deliver", Label: "B1", Time: 12}); err == nil {
		t.Fatal("delivering before final airport must fail")
	}
	if r, err := s.ApplyScan(ScanEvent{ID: 4, Type: "load", Label: "B1", Time: 13, Flight: "F1"}); err != nil || r.Loaded != "F1" {
		t.Fatalf("valid load after failures: result=%+v err=%v", r, err)
	}
	if _, err := s.ApplyScan(ScanEvent{ID: 5, Type: "unload", Label: "B1", Time: 13, Flight: "F1"}); err == nil {
		t.Fatal("equal timestamp must fail strict-increasing check")
	}
	if _, err := s.ApplyScan(ScanEvent{ID: 6, Type: "load", Label: "B1", Time: 14, Flight: "F1"}); err == nil {
		t.Fatal("loading while already loaded must fail")
	}
}

func TestDuplicateEvents(t *testing.T) {
	s := NewState()
	mustRegister(t, s, Flight{Number: "F1", From: "A", To: "B"})
	if err := s.RegisterBag("B1", []Segment{{Flight: "F1", From: "A", To: "B"}}); err != nil {
		t.Fatal(err)
	}
	ev := ScanEvent{ID: 1, Type: "load", Label: "B1", Time: 1, Flight: "F1"}
	first, err := s.ApplyScan(ev)
	if err != nil {
		t.Fatal(err)
	}
	cached, known, match := s.LookupEvent(ev)
	if !known || !match || cached == nil {
		t.Fatal("identical event should replay")
	}
	if cached.Replayed != true || cached.Location != first.Location {
		t.Fatalf("replayed result mismatch: %+v", cached)
	}
	if len(s.Bags["B1"].Scans) != 1 {
		t.Fatal("replayed event must not append another scan")
	}
	if _, _, match := s.LookupEvent(ScanEvent{ID: 1, Type: "load", Label: "B1", Time: 2, Flight: "F1"}); match {
		t.Fatal("same id with different time must not match")
	}
}

func TestRemoveAndRebook(t *testing.T) {
	s := NewState()
	mustRegister(t, s,
		Flight{Number: "F1", From: "A", To: "B"},
		Flight{Number: "F2", From: "B", To: "C"},
		Flight{Number: "F3", From: "A", To: "D"},
		Flight{Number: "F4", From: "D", To: "C"},
	)
	if err := s.RegisterBag("B1", []Segment{
		{Flight: "F1", From: "A", To: "B"},
		{Flight: "F2", From: "B", To: "C"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ApplyScan(ScanEvent{ID: 1, Type: "load", Label: "B1", Time: 1, Flight: "F1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rebook(RebookRequest{Label: "B1", Segments: []Segment{{Flight: "F3", From: "A", To: "D"}}}); err == nil {
		t.Fatal("rebook while loaded must fail")
	}
	r, err := s.ApplyScan(ScanEvent{ID: 2, Type: "remove", Label: "B1", Time: 2, Flight: "F1"})
	if err != nil || r.Location != "A" || r.Loaded != "" || len(r.Completed) != 0 {
		t.Fatalf("remove result: %+v err=%v", r, err)
	}

	if remaining, err := s.Rebook(RebookRequest{Label: "B1", Segments: []Segment{
		{Flight: "F1", From: "A", To: "B"},
	}}); err != nil || len(remaining) != 1 {
		t.Fatalf("re-checking the removed (not completed) flight should be allowed: %v err=%v", remaining, err)
	}

	remaining, err := s.Rebook(RebookRequest{Label: "B1", Segments: []Segment{
		{Flight: "F3", From: "A", To: "D"},
		{Flight: "F4", From: "D", To: "C"},
	}})
	if err != nil || len(remaining) != 2 {
		t.Fatalf("rebook: %v err=%v", remaining, err)
	}

	if _, err := s.ApplyScan(ScanEvent{ID: 3, Type: "load", Label: "B1", Time: 3, Flight: "F1"}); err == nil {
		t.Fatal("completed-plan rule: old flight must never reload after rebook removes it")
	}
	for _, ev := range []ScanEvent{
		{ID: 4, Type: "load", Label: "B1", Time: 4, Flight: "F3"},
		{ID: 5, Type: "unload", Label: "B1", Time: 5, Flight: "F3"},
		{ID: 6, Type: "load", Label: "B1", Time: 6, Flight: "F4"},
		{ID: 7, Type: "unload", Label: "B1", Time: 7, Flight: "F4"},
		{ID: 8, Type: "deliver", Label: "B1", Time: 8},
	} {
		if _, err := s.ApplyScan(ev); err != nil {
			t.Fatalf("apply %+v: %v", ev, err)
		}
	}
	if _, err := s.Rebook(RebookRequest{Label: "B1", Segments: []Segment{{Flight: "F2", From: "C", To: "X"}}}); err == nil {
		t.Fatal("rebook after delivery must fail")
	}
}

func TestRebookAfterUnloadKeepsHistory(t *testing.T) {
	s := NewState()
	mustRegister(t, s,
		Flight{Number: "F1", From: "A", To: "B"},
		Flight{Number: "F2", From: "B", To: "C"},
		Flight{Number: "F5", From: "B", To: "E"},
	)
	if err := s.RegisterBag("B1", []Segment{
		{Flight: "F1", From: "A", To: "B"},
		{Flight: "F2", From: "B", To: "C"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []ScanEvent{
		{ID: 1, Type: "load", Label: "B1", Time: 1, Flight: "F1"},
		{ID: 2, Type: "unload", Label: "B1", Time: 2, Flight: "F1"},
	} {
		if _, err := s.ApplyScan(ev); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Rebook(RebookRequest{Label: "B1", Segments: []Segment{{Flight: "F1", From: "B", To: "C"}}}); err == nil {
		t.Fatal("rebook must not reuse a completed flight")
	}
	if _, err := s.Rebook(RebookRequest{Label: "B1", Segments: []Segment{{Flight: "F5", From: "A", To: "E"}}}); err == nil {
		t.Fatal("new plan must start at current location B")
	}
	if _, err := s.Rebook(RebookRequest{Label: "B1", Segments: []Segment{{Flight: "F5", From: "B", To: "E"}}}); err != nil {
		t.Fatalf("valid rebook at transfer airport: %v", err)
	}
	view, _ := s.Query("B1")
	if len(view.Completed) != 1 || view.Completed[0].Flight != "F1" || view.Location != "B" {
		t.Fatalf("history/location wrong: %+v", view)
	}
}
