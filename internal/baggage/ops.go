package baggage

import "fmt"

const (
	scanLoad      = "load"
	scanUnload    = "unload"
	scanRemove    = "remove"
	scanDelivered = "delivered"
)

// applyScan validates and applies one scan event. It returns the stored scan
// record and an error describing the rejection.
func (s *Store) applyScan(bag *Bag, scanType, flightID string, time int64) (Scan, error) {
	if bag.Delivered {
		return Scan{}, fmt.Errorf("bag %s already delivered, no further scans allowed", bag.Label)
	}
	if len(bag.Scans) > 0 && time <= bag.LastTime {
		return Scan{}, fmt.Errorf("scan time %d must be strictly after previous time %d", time, bag.LastTime)
	}

	switch scanType {
	case scanLoad:
		if err := s.checkLoad(bag, flightID); err != nil {
			return Scan{}, err
		}
		bag.Loaded = flightID
		bag.Location = ""
	case scanUnload:
		f, ok := s.flights[flightID]
		if !ok {
			return Scan{}, fmt.Errorf("unknown flight %s", flightID)
		}
		if bag.Loaded != flightID {
			return Scan{}, fmt.Errorf("bag %s is not loaded on flight %s", bag.Label, flightID)
		}
		completed := bag.Remaining[0]
		bag.Completed = append(bag.Completed, completed)
		bag.Remaining = bag.Remaining[1:]
		bag.Loaded = ""
		bag.Location = f.To
	case scanRemove:
		f, ok := s.flights[flightID]
		if !ok {
			return Scan{}, fmt.Errorf("unknown flight %s", flightID)
		}
		if bag.Loaded != flightID {
			return Scan{}, fmt.Errorf("bag %s is not loaded on flight %s", bag.Label, flightID)
		}
		bag.Loaded = ""
		bag.Location = f.From
	case scanDelivered:
		if flightID != "" {
			return Scan{}, fmt.Errorf("delivery scan must not name a flight")
		}
		if len(bag.Remaining) != 0 {
			return Scan{}, fmt.Errorf("bag %s has not reached its final airport", bag.Label)
		}
		bag.Delivered = true
	default:
		return Scan{}, fmt.Errorf("unknown scan type %q", scanType)
	}

	rec := Scan{Event: "", Type: scanType, Flight: flightID, Time: time}
	bag.LastTime = time
	bag.Scans = append(bag.Scans, rec)
	return rec, nil
}

func (s *Store) checkLoad(bag *Bag, flightID string) error {
	f, ok := s.flights[flightID]
	if !ok {
		return fmt.Errorf("unknown flight %s", flightID)
	}
	if bag.Loaded != "" {
		return fmt.Errorf("bag %s is already loaded on %s", bag.Label, bag.Loaded)
	}
	if len(bag.Remaining) == 0 {
		return fmt.Errorf("bag %s has no remaining segments", bag.Label)
	}
	next := bag.Remaining[0]
	if flightID != next {
		return fmt.Errorf("bag %s can only be loaded onto next segment %s at %s", bag.Label, next, bag.Location)
	}
	if bag.Location != f.From {
		return fmt.Errorf("bag %s is at %s, flight %s departs from %s", bag.Label, bag.Location, flightID, f.From)
	}
	return nil
}

// rebook replaces the remaining plan of a ground bag. Completed history is
// preserved and the new plan must connect from the current location without
// repeating flights (including completed ones).
func (s *Store) rebook(bag *Bag, newSegments []string) error {
	if bag.Delivered {
		return fmt.Errorf("bag %s already delivered and cannot be rebooked", bag.Label)
	}
	if bag.Loaded != "" {
		return fmt.Errorf("bag %s is loaded on %s, unload or remove it before rebooking", bag.Label, bag.Loaded)
	}
	if len(newSegments) == 0 {
		return fmt.Errorf("new plan for bag %s must be non-empty", bag.Label)
	}
	used := map[string]bool{}
	for _, id := range bag.Completed {
		used[id] = true
	}
	var prevAirport string
	for i, id := range newSegments {
		if id == "" {
			return fmt.Errorf("bag %s has an empty segment", bag.Label)
		}
		if used[id] {
			return fmt.Errorf("bag %s cannot reuse completed or repeated flight %s", bag.Label, id)
		}
		used[id] = true
		f, ok := s.flights[id]
		if !ok {
			return fmt.Errorf("bag %s references unknown flight %s", bag.Label, id)
		}
		if i == 0 {
			if f.From != bag.Location {
				return fmt.Errorf("new plan for bag %s must start at current location %s, but %s departs %s",
					bag.Label, bag.Location, f.ID, f.From)
			}
		} else if prevAirport != f.From {
			return fmt.Errorf("new plan for bag %s does not connect at %s", bag.Label, prevAirport)
		}
		prevAirport = f.To
	}
	bag.Remaining = append([]string(nil), newSegments...)
	bag.Segments = append(append([]string(nil), bag.Completed...), newSegments...)
	return nil
}
