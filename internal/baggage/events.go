package baggage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ScanEvent is an inbound scan request for one bag.
type ScanEvent struct {
	ID     int    `json:"id"`
	Type   string `json:"type"`
	Label  string `json:"label"`
	Time   int64  `json:"time"`
	Flight string `json:"flight,omitempty"`
}

// ScanResult is returned after an accepted scan.
type ScanResult struct {
	Status    string    `json:"status"`
	Replayed  bool      `json:"replayed"`
	Label     string    `json:"label"`
	Location  string    `json:"location"`
	Loaded    string    `json:"loaded,omitempty"`
	Delivered bool      `json:"delivered"`
	Completed []Segment `json:"completed"`
	Remaining []Segment `json:"remaining"`
}

func canonicalHash(e ScanEvent) string {
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LookupEvent returns the cached response for a previously accepted event id.
// It reports whether the id is known and whether the incoming event matches it.
func (s *State) LookupEvent(e ScanEvent) (*ScanResult, bool, bool) {
	cached, ok := s.Events[e.ID]
	if !ok {
		return nil, false, false
	}
	if cached.Hash != canonicalHash(e) {
		return nil, true, false
	}
	var result ScanResult
	_ = json.Unmarshal(cached.Body, &result)
	result.Replayed = true
	return &result, true, true
}

// ApplyScan validates and applies one scan event.
func (s *State) ApplyScan(e ScanEvent) (*ScanResult, error) {
	bag, ok := s.Bags[e.Label]
	if !ok {
		return nil, fmt.Errorf("bag %q is not registered", e.Label)
	}
	if bag.Delivered {
		return nil, fmt.Errorf("bag %q has already been delivered", e.Label)
	}
	if bag.LastTime != nil && e.Time <= *bag.LastTime {
		return nil, fmt.Errorf("scan time must be strictly greater than the previous scan time %d for bag %q", *bag.LastTime, e.Label)
	}

	switch e.Type {
	case "load":
		if err := bag.load(e.Flight, s.Flights); err != nil {
			return nil, err
		}
	case "unload":
		if err := bag.unload(e.Flight, s.Flights); err != nil {
			return nil, err
		}
	case "remove":
		if err := bag.remove(e.Flight, s.Flights); err != nil {
			return nil, err
		}
	case "deliver":
		if err := bag.deliver(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown scan type %q", e.Type)
	}

	last := e.Time
	bag.LastTime = &last
	bag.Scans = append(bag.Scans, Scan{ID: e.ID, Type: e.Type, Time: e.Time, Flight: e.Flight})

	result := bag.snapshot("ok", false)
	body, _ := json.Marshal(result)
	s.Events[e.ID] = CachedEvent{Hash: canonicalHash(e), Body: body}
	return result, nil
}

func (b *Bag) load(flightNo string, flights map[string]Flight) error {
	if flightNo == "" {
		return fmt.Errorf("load scan requires a flight number")
	}
	flight, ok := flights[flightNo]
	if !ok {
		return fmt.Errorf("flight %q is not registered", flightNo)
	}
	if b.Loaded != "" {
		return fmt.Errorf("bag %q is already loaded on flight %q", b.Label, b.Loaded)
	}
	if len(b.Remaining) == 0 {
		return fmt.Errorf("bag %q has no remaining planned segments", b.Label)
	}
	next := b.Remaining[0]
	if next.Flight != flightNo {
		return fmt.Errorf("flight %q is not the next planned flight for bag %q (expected %q)", flightNo, b.Label, next.Flight)
	}
	if b.Location != flight.From {
		return fmt.Errorf("bag %q is at %s but flight %q departs from %s", b.Label, b.Location, flightNo, flight.From)
	}
	b.Loaded = flightNo
	return nil
}

func (b *Bag) unload(flightNo string, flights map[string]Flight) error {
	if flightNo == "" {
		return fmt.Errorf("unload scan requires a flight number")
	}
	if _, ok := flights[flightNo]; !ok {
		return fmt.Errorf("flight %q is not registered", flightNo)
	}
	if b.Loaded == "" {
		return fmt.Errorf("bag %q is not loaded on any flight", b.Label)
	}
	if b.Loaded != flightNo {
		return fmt.Errorf("bag %q is loaded on flight %q, not %q", b.Label, b.Loaded, flightNo)
	}
	seg := b.Remaining[0]
	b.Completed = append(b.Completed, seg)
	b.Remaining = b.Remaining[1:]
	b.Location = seg.To
	b.Loaded = ""
	return nil
}

func (b *Bag) remove(flightNo string, flights map[string]Flight) error {
	if flightNo == "" {
		return fmt.Errorf("remove scan requires a flight number")
	}
	flight, ok := flights[flightNo]
	if !ok {
		return fmt.Errorf("flight %q is not registered", flightNo)
	}
	if b.Loaded == "" {
		return fmt.Errorf("bag %q is not loaded on any flight", b.Label)
	}
	if b.Loaded != flightNo {
		return fmt.Errorf("bag %q is loaded on flight %q, not %q", b.Label, b.Loaded, flightNo)
	}
	b.Loaded = ""
	b.Location = flight.From
	return nil
}

func (b *Bag) deliver() error {
	if b.Loaded != "" {
		return fmt.Errorf("bag %q is still loaded on flight %q", b.Label, b.Loaded)
	}
	if len(b.Remaining) != 0 {
		return fmt.Errorf("bag %q has not reached its final airport", b.Label)
	}
	b.Delivered = true
	return nil
}

func (b *Bag) snapshot(status string, replayed bool) *ScanResult {
	completed := make([]Segment, len(b.Completed))
	copy(completed, b.Completed)
	remaining := make([]Segment, len(b.Remaining))
	copy(remaining, b.Remaining)
	return &ScanResult{
		Status:    status,
		Replayed:  replayed,
		Label:     b.Label,
		Location:  b.Location,
		Loaded:    b.Loaded,
		Delivered: b.Delivered,
		Completed: completed,
		Remaining: remaining,
	}
}
