package baggage

import (
	"encoding/json"
	"fmt"
)

// Flight describes a scheduled non-stop flight between two airports.
type Flight struct {
	Number string `json:"number"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Segment is one planned interline leg flown on a specific flight.
type Segment struct {
	Flight string `json:"flight"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Scan is a recorded airport scanning event for one bag.
type Scan struct {
	ID     int    `json:"id"`
	Type   string `json:"type"`
	Time   int64  `json:"time"`
	Flight string `json:"flight,omitempty"`
}

// Bag tracks one piece of luggage through its itinerary.
type Bag struct {
	Label     string    `json:"label"`
	Completed []Segment `json:"completed"`
	Remaining []Segment `json:"remaining"`
	// Loaded is the flight number the bag is currently loaded on, "" when on the ground.
	Loaded    string `json:"loaded,omitempty"`
	Location  string `json:"location"`
	Delivered bool   `json:"delivered"`
	LastTime  *int64 `json:"last_time,omitempty"`
	Scans     []Scan `json:"scans"`
}

// CachedEvent stores the idempotent response for an accepted scan event id.
type CachedEvent struct {
	Hash string          `json:"hash"`
	Body json.RawMessage `json:"body"`
}

// State is the full persisted tracker state.
type State struct {
	Flights map[string]Flight   `json:"flights"`
	Bags    map[string]*Bag     `json:"bags"`
	Events  map[int]CachedEvent `json:"events"`
}

// NewState returns an empty tracker state.
func NewState() *State {
	return &State{
		Flights: map[string]Flight{},
		Bags:    map[string]*Bag{},
		Events:  map[int]CachedEvent{},
	}
}

// RegisterFlight registers a flight number with distinct departure and arrival airports.
func (s *State) RegisterFlight(f Flight) error {
	if f.Number == "" {
		return fmt.Errorf("flight number must not be empty")
	}
	if f.From == "" || f.To == "" {
		return fmt.Errorf("flight %q airports must not be empty", f.Number)
	}
	if f.From == f.To {
		return fmt.Errorf("flight %q departure and arrival airports must differ", f.Number)
	}
	if existing, ok := s.Flights[f.Number]; ok {
		if existing != f {
			return fmt.Errorf("flight %q already registered with different details", f.Number)
		}
		return nil
	}
	s.Flights[f.Number] = f
	return nil
}

// RegisterBag checks in a bag with a non-empty, connected, flight-unique itinerary.
func (s *State) RegisterBag(label string, segments []Segment) error {
	if label == "" {
		return fmt.Errorf("bag label must not be empty")
	}
	if _, ok := s.Bags[label]; ok {
		return fmt.Errorf("bag %q already registered", label)
	}
	if len(segments) == 0 {
		return fmt.Errorf("bag %q itinerary must not be empty", label)
	}
	seen := map[string]bool{}
	for i, seg := range segments {
		flight, ok := s.Flights[seg.Flight]
		if !ok {
			return fmt.Errorf("segment %d: flight %q is not registered", i, seg.Flight)
		}
		if seg.From != flight.From || seg.To != flight.To {
			return fmt.Errorf("segment %d: flight %q does not fly %s -> %s", i, seg.Flight, seg.From, seg.To)
		}
		if seen[seg.Flight] {
			return fmt.Errorf("segment %d: flight %q is repeated in the itinerary", i, seg.Flight)
		}
		seen[seg.Flight] = true
		if i > 0 && segments[i-1].To != seg.From {
			return fmt.Errorf("segment %d is not connected to the previous segment", i)
		}
	}
	segs := make([]Segment, len(segments))
	copy(segs, segments)
	s.Bags[label] = &Bag{
		Label:     label,
		Remaining: segs,
		Location:  segs[0].From,
		Completed: []Segment{},
		Scans:     []Scan{},
	}
	return nil
}
