package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/interline-baggage-tracker/internal/baggage"
)

type envelope struct {
	Op       string            `json:"op"`
	Number   string            `json:"number,omitempty"`
	From     string            `json:"from,omitempty"`
	To       string            `json:"to,omitempty"`
	Label    string            `json:"label,omitempty"`
	Segments []baggage.Segment `json:"segments,omitempty"`
	ID       *int              `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Time     int64             `json:"time,omitempty"`
	Flight   string            `json:"flight,omitempty"`
}

func ok(v any) map[string]any { return map[string]any{"ok": true, "result": v} }
func fail(err error) map[string]any {
	return map[string]any{"ok": false, "error": err.Error()}
}

func main() {
	stateDir := flag.String("state", "", "directory for persisted tracker state")
	flag.Parse()
	if *stateDir == "" {
		fmt.Fprintln(os.Stderr, "usage: baggage --state <state-directory>")
		os.Exit(2)
	}
	if err := run(*stateDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stateDir string) error {
	state, err := baggage.Load(stateDir)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	enc := json.NewEncoder(out)

	emit := func(v map[string]any) {
		if err := enc.Encode(v); err != nil {
			fmt.Fprintln(os.Stderr, "write response:", err)
		}
		out.Flush()
	}

	for in.Scan() {
		line := in.Bytes()
		if len(bytesTrimSpace(line)) == 0 {
			continue
		}
		var req envelope
		if err := json.Unmarshal(line, &req); err != nil {
			emit(fail(fmt.Errorf("invalid json: %w", err)))
			continue
		}

		changed, result, err := dispatch(state, &req)
		if err != nil {
			emit(fail(err))
			continue
		}
		if changed {
			if err := baggage.Save(stateDir, state); err != nil {
				emit(fail(fmt.Errorf("save state: %w", err)))
				continue
			}
		}
		emit(ok(result))
	}
	return in.Err()
}

func bytesTrimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}

func dispatch(state *baggage.State, req *envelope) (bool, any, error) {
	switch req.Op {
	case "register-flight":
		err := state.RegisterFlight(baggage.Flight{Number: req.Number, From: req.From, To: req.To})
		return err == nil, map[string]string{"registered": req.Number}, err
	case "register-bag":
		err := state.RegisterBag(req.Label, req.Segments)
		return err == nil, map[string]any{
			"label":    req.Label,
			"location": firstFrom(req.Segments),
		}, err
	case "scan":
		if req.ID == nil {
			return false, nil, fmt.Errorf("scan requires an id")
		}
		event := baggage.ScanEvent{
			ID:     *req.ID,
			Type:   req.Type,
			Label:  req.Label,
			Time:   req.Time,
			Flight: req.Flight,
		}
		if cached, known, match := state.LookupEvent(event); known {
			if !match {
				return false, nil, fmt.Errorf("event id %d already used with different content", event.ID)
			}
			return false, cached, nil
		}
		result, err := state.ApplyScan(event)
		return err == nil, result, err
	case "rebook":
		remaining, err := state.Rebook(baggage.RebookRequest{Label: req.Label, Segments: req.Segments})
		return err == nil, map[string]any{"label": req.Label, "remaining": remaining}, err
	case "query":
		view, err := state.Query(req.Label)
		return false, view, err
	case "query-all":
		return false, state.QueryAll(), nil
	default:
		return false, nil, fmt.Errorf("unknown op %q", req.Op)
	}
}

func firstFrom(segs []baggage.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	return segs[0].From
}
