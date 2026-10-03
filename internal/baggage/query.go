package baggage

import (
	"fmt"
	"sort"
)

// RebookRequest replaces the remaining itinerary of a bag on the ground.
type RebookRequest struct {
	Label    string    `json:"label"`
	Segments []Segment `json:"segments"`
}

// Rebook replaces the unflown portion of a bag's plan. Completed history is kept.
func (s *State) Rebook(req RebookRequest) ([]Segment, error) {
	bag, ok := s.Bags[req.Label]
	if !ok {
		return nil, fmt.Errorf("bag %q is not registered", req.Label)
	}
	if bag.Delivered {
		return nil, fmt.Errorf("bag %q has already been delivered and cannot be rebooked", req.Label)
	}
	if bag.Loaded != "" {
		return nil, fmt.Errorf("bag %q is loaded on flight %q and cannot be rebooked", req.Label, bag.Loaded)
	}
	if len(req.Segments) == 0 {
		return nil, fmt.Errorf("rebooked itinerary for bag %q must not be empty", req.Label)
	}

	done := map[string]bool{}
	for _, seg := range bag.Completed {
		done[seg.Flight] = true
	}

	seen := map[string]bool{}
	for i, seg := range req.Segments {
		flight, ok := s.Flights[seg.Flight]
		if !ok {
			return nil, fmt.Errorf("segment %d: flight %q is not registered", i, seg.Flight)
		}
		if seg.From != flight.From || seg.To != flight.To {
			return nil, fmt.Errorf("segment %d: flight %q does not fly %s -> %s", i, seg.Flight, seg.From, seg.To)
		}
		if seen[seg.Flight] {
			return nil, fmt.Errorf("segment %d: flight %q is repeated", i, seg.Flight)
		}
		if done[seg.Flight] {
			return nil, fmt.Errorf("segment %d: flight %q has already been completed", i, seg.Flight)
		}
		seen[seg.Flight] = true
		if i == 0 {
			if seg.From != bag.Location {
				return nil, fmt.Errorf("new itinerary must start at the bag's current location %s", bag.Location)
			}
		} else if req.Segments[i-1].To != seg.From {
			return nil, fmt.Errorf("segment %d is not connected to the previous segment", i)
		}
	}

	segs := make([]Segment, len(req.Segments))
	copy(segs, req.Segments)
	bag.Remaining = segs
	out := make([]Segment, len(segs))
	copy(out, segs)
	return out, nil
}

// BagView is the query projection for one bag.
type BagView struct {
	Label      string    `json:"label"`
	Location   string    `json:"location"`
	Loaded     string    `json:"loaded,omitempty"`
	Delivered  bool      `json:"delivered"`
	NextAction string    `json:"next_action"`
	Completed  []Segment `json:"completed"`
	Remaining  []Segment `json:"remaining"`
	Scans      []Scan    `json:"scans"`
}

// Query returns the current projection of one bag.
func (s *State) Query(label string) (*BagView, error) {
	bag, ok := s.Bags[label]
	if !ok {
		return nil, fmt.Errorf("bag %q is not registered", label)
	}
	return viewOf(bag), nil
}

// QueryAll returns projections of every bag, sorted by label.
func (s *State) QueryAll() []*BagView {
	labels := make([]string, 0, len(s.Bags))
	for label := range s.Bags {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	views := make([]*BagView, 0, len(labels))
	for _, label := range labels {
		views = append(views, viewOf(s.Bags[label]))
	}
	return views
}

func viewOf(b *Bag) *BagView {
	completed := make([]Segment, len(b.Completed))
	copy(completed, b.Completed)
	remaining := make([]Segment, len(b.Remaining))
	copy(remaining, b.Remaining)

	scans := make([]Scan, len(b.Scans))
	copy(scans, b.Scans)
	sort.SliceStable(scans, func(i, j int) bool { return scans[i].Time < scans[j].Time })

	return &BagView{
		Label:      b.Label,
		Location:   b.Location,
		Loaded:     b.Loaded,
		Delivered:  b.Delivered,
		NextAction: nextAction(b),
		Completed:  completed,
		Remaining:  remaining,
		Scans:      scans,
	}
}

func nextAction(b *Bag) string {
	switch {
	case b.Delivered:
		return "none: bag delivered"
	case b.Loaded != "":
		return "unload flight " + b.Loaded + " at arrival airport"
	case len(b.Remaining) > 0:
		next := b.Remaining[0]
		return "load onto flight " + next.Flight + " at " + b.Location
	default:
		return "deliver to claimant at " + b.Location
	}
}
