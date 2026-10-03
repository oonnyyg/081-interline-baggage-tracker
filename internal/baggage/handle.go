package baggage

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// Handle processes one decoded request line and always returns a response map.
// Invalid requests leave the store unchanged.
func (s *Store) Handle(req map[string]any) map[string]any {
	op, _ := req["op"].(string)
	switch op {
	case "flight":
		return s.handleFlight(req)
	case "bag":
		return s.handleBag(req)
	case "scan":
		return s.handleScan(req)
	case "rebook":
		return s.handleRebook(req)
	case "query":
		return s.handleQuery(req)
	default:
		return errResp(fmt.Sprintf("unknown or missing op %q", op))
	}
}

func errResp(msg string) map[string]any {
	return map[string]any{"ok": false, "error": msg}
}

func strField(req map[string]any, key string) (string, bool) {
	v, ok := req[key]
	if !ok {
		return "", false
	}
	str, ok := v.(string)
	return str, ok
}

func intField(req map[string]any, key string) (int64, bool) {
	v, ok := req[key]
	if !ok {
		return 0, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		num := rv.Float()
		if num != float64(int64(num)) {
			return 0, false
		}
		return int64(num), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), true
	}
	return 0, false
}

func segmentsField(req map[string]any) ([]string, bool) {
	v, ok := req["segments"]
	if !ok {
		return nil, false
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		str, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, str)
	}
	return out, true
}

func (s *Store) handleFlight(req map[string]any) map[string]any {
	id, ok1 := strField(req, "id")
	from, ok2 := strField(req, "from")
	to, ok3 := strField(req, "to")
	if !ok1 || !ok2 || !ok3 {
		return errResp("flight requires string fields id, from, to")
	}
	if err := s.addFlight(Flight{ID: id, From: from, To: to}); err != nil {
		return errResp(err.Error())
	}
	return map[string]any{"ok": true, "op": "flight", "id": id}
}

func (s *Store) handleBag(req map[string]any) map[string]any {
	label, ok := strField(req, "label")
	if !ok {
		return errResp("bag requires string field label")
	}
	segments, ok := segmentsField(req)
	if !ok {
		return errResp("bag requires a segments array of flight ids")
	}
	if _, err := s.registerBag(label, segments); err != nil {
		return errResp(err.Error())
	}
	return map[string]any{"ok": true, "op": "bag", "label": label}
}

func (s *Store) handleScan(req map[string]any) map[string]any {
	eventID, ok1 := strField(req, "event")
	label, ok2 := strField(req, "label")
	scanType, ok3 := strField(req, "type")
	time, ok4 := intField(req, "time")
	if !ok1 || eventID == "" || !ok2 || !ok3 || !ok4 {
		return errResp("scan requires non-empty string event, label, type and integer time")
	}
	if prev, dup := s.events[eventID]; dup {
		if !sameRequest(prev.request, cloneMap(req)) {
			return errResp(fmt.Sprintf("event %s already used with different content", eventID))
		}
		return prev.result
	}
	flight, _ := strField(req, "flight")
	bag, err := s.bag(label)
	if err != nil {
		return errResp(err.Error())
	}
	rec, err := s.applyScan(bag, scanType, flight, time)
	if err != nil {
		return errResp(err.Error())
	}
	rec.Event = eventID
	bag.Scans[len(bag.Scans)-1] = rec

	result := map[string]any{
		"ok":     true,
		"op":     "scan",
		"event":  eventID,
		"label":  label,
		"type":   scanType,
		"time":   time,
		"status": s.statusView(bag),
	}
	if flight != "" {
		result["flight"] = flight
	}
	s.events[eventID] = eventRecord{request: cloneMap(req), result: result}
	return result
}

func (s *Store) handleRebook(req map[string]any) map[string]any {
	label, ok := strField(req, "label")
	if !ok {
		return errResp("rebook requires string field label")
	}
	segments, ok := segmentsField(req)
	if !ok {
		return errResp("rebook requires a segments array of flight ids")
	}
	bag, err := s.bag(label)
	if err != nil {
		return errResp(err.Error())
	}
	if err := s.rebook(bag, segments); err != nil {
		return errResp(err.Error())
	}
	return map[string]any{
		"ok":        true,
		"op":        "rebook",
		"label":     label,
		"completed": append([]string(nil), bag.Completed...),
		"remaining": append([]string(nil), bag.Remaining...),
	}
}

func (s *Store) statusView(bag *Bag) map[string]any {
	st := map[string]any{}
	switch {
	case bag.Delivered:
		st["state"] = "delivered"
		st["location"] = bag.Location
	case bag.Loaded != "":
		st["state"] = "loaded"
		st["flight"] = bag.Loaded
	default:
		st["state"] = "ground"
		st["location"] = bag.Location
	}
	return st
}

func (s *Store) handleQuery(req map[string]any) map[string]any {
	label, ok := strField(req, "label")
	if !ok {
		return errResp("query requires string field label")
	}
	bag, err := s.bag(label)
	if err != nil {
		return errResp(err.Error())
	}
	scans := append([]Scan(nil), bag.Scans...)
	sort.SliceStable(scans, func(i, j int) bool { return scans[i].Time < scans[j].Time })
	return map[string]any{
		"ok":        true,
		"op":        "query",
		"label":     bag.Label,
		"status":    s.statusView(bag),
		"plan":      append([]string(nil), bag.Segments...),
		"completed": append([]string(nil), bag.Completed...),
		"remaining": append([]string(nil), bag.Remaining...),
		"scans":     scans,
	}
}

func cloneMap(in map[string]any) map[string]any {
	data, err := json.Marshal(in)
	if err != nil {
		out := make(map[string]any, len(in))
		for k, v := range in {
			out[k] = v
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		out = map[string]any{}
	}
	return out
}
