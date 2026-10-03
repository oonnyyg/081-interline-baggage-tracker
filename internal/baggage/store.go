package baggage

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateFileName = "state.json"

// Load reads the state from dir, creating an empty one if it does not exist.
func Load(dir string) (*State, error) {
	path := filepath.Join(dir, stateFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewState(), nil
	}
	if err != nil {
		return nil, err
	}
	state := NewState()
	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}
	if state.Flights == nil {
		state.Flights = map[string]Flight{}
	}
	if state.Bags == nil {
		state.Bags = map[string]*Bag{}
	}
	if state.Events == nil {
		state.Events = map[int]CachedEvent{}
	}
	return state, nil
}

// Save atomically persists the state into dir.
func Save(dir string, state *State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(dir, stateFileName)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}
