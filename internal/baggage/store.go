package baggage

import (
	"fmt"
	"reflect"
	"sort"
)

// Flight describes a scheduled non-stop flight between two airports.
type Flight struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Scan is a recorded scan event for a bag, ordered by time.
type Scan struct {
	Event  string `json:"event"`
	Type   string `json:"type"`
	Flight string `json:"flight,omitempty"`
	Time   int64  `json:"time"`
}

// Bag holds the tracking state of one checked bag.
type Bag struct {
	Label     string   `json:"label"`
	Segments  []string `json:"segments"`
	Completed []string `json:"completed"`
	Remaining []string `json:"remaining"`
	Location  string   `json:"location,omitempty"`
	Loaded    string   `json:"loaded,omitempty"`
	Delivered bool     `json:"delivered"`
	LastTime  int64    `json:"last_time"`
	Scans     []Scan   `json:"scans"`
}

type eventRecord struct {
	request map[string]any
	result  map[string]any
}

// Store is the in-memory tracking state. It is safe for sequential use by a
// single processor; callers must serialize Handle calls.
type Store struct {
	flights map[string]Flight
	bags    map[string]*Bag
	events  map[string]eventRecord
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		flights: map[string]Flight{},
		bags:    map[string]*Bag{},
		events:  map[string]eventRecord{},
	}
}

func (s *Store) addFlight(f Flight) error {
	if f.ID == "" {
		return fmt.Errorf("flight id is required")
	}
	if f.From == "" || f.To == "" {
		return fmt.Errorf("flight airports are required")
	}
	if f.From == f.To {
		return fmt.Errorf("flight %s departure and arrival must differ", f.ID)
	}
	if _, exists := s.flights[f.ID]; exists {
		return fmt.Errorf("flight %s already registered", f.ID)
	}
	s.flights[f.ID] = f
	return nil
}

func (s *Store) registerBag(label string, segments []string) (*Bag, error) {
	if label == "" {
		return nil, fmt.Errorf("bag label is required")
	}
	if _, exists := s.bags[label]; exists {
		return nil, fmt.Errorf("bag %s already registered", label)
	}
	bag, err := s.planBag(label, segments, nil, "")
	if err != nil {
		return nil, err
	}
	s.bags[label] = bag
	return bag, nil
}

// planBag validates a segment list and builds the resulting bag state. When
// completed is non-nil the bag starts with that history at currentLocation.
func (s *Store) planBag(label string, segments []string, completed []string, currentLocation string) (*Bag, error) {
	if len(segments) == 0 {
		return nil, fmt.Errorf("bag %s requires at least one segment", label)
	}
	seen := map[string]bool{}
	var prev Flight
	for i, id := range segments {
		if id == "" {
			return nil, fmt.Errorf("bag %s has an empty segment", label)
		}
		if seen[id] {
			return nil, fmt.Errorf("bag %s repeats flight %s", label, id)
		}
		seen[id] = true
		f, ok := s.flights[id]
		if !ok {
			return nil, fmt.Errorf("bag %s references unknown flight %s", label, id)
		}
		if i > 0 && prev.To != f.From {
			return nil, fmt.Errorf("bag %s segments do not connect: %s arrives at %s but %s departs %s",
				label, prev.ID, prev.To, f.ID, f.From)
		}
		prev = f
	}
	first := s.flights[segments[0]]
	location := first.From
	if currentLocation != "" {
		location = currentLocation
	}
	rem := append([]string(nil), segments...)
	var done []string
	if completed != nil {
		done = append([]string(nil), completed...)
	}
	return &Bag{
		Label:     label,
		Segments:  append([]string(nil), segments...),
		Completed: done,
		Remaining: rem,
		Location:  location,
	}, nil
}

func (s *Store) bag(label string) (*Bag, error) {
	bag, ok := s.bags[label]
	if !ok {
		return nil, fmt.Errorf("unknown bag %s", label)
	}
	return bag, nil
}

// SortedBagLabels returns bag labels in registration order-independent sorted
// order; currently used only by tests and tooling.
func (s *Store) SortedBagLabels() []string {
	labels := make([]string, 0, len(s.bags))
	for l := range s.bags {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return labels
}

func sameRequest(a, b map[string]any) bool {
	return reflect.DeepEqual(a, b)
}
