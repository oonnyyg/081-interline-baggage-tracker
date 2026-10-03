package baggage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const journalName = "journal.jsonl"

// Processor owns a Store backed by an append-only JSONL journal.
type Processor struct {
	store *Store
	file  *os.File
	enc   *json.Encoder
}

// OpenProcessor opens (creating the directory if needed) and replays the
// journal so tracking continues where it left off.
func OpenProcessor(stateDir string) (*Processor, error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	path := filepath.Join(stateDir, journalName)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	p := &Processor{store: NewStore(), file: f}
	if err := p.replay(); err != nil {
		f.Close()
		return nil, err
	}
	p.enc = json.NewEncoder(f)
	return p, nil
}

type journalLine struct {
	Request map[string]any `json:"request"`
}

func (p *Processor) replay() error {
	scanner := bufio.NewScanner(p.file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry journalLine
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("journal line %d is corrupt: %w", lineNo, err)
		}
		resp := p.store.Handle(entry.Request)
		if ok, _ := resp["ok"].(bool); !ok {
			return fmt.Errorf("journal line %d no longer applies: %v", lineNo, resp["error"])
		}
	}
	return scanner.Err()
}

// Process handles one request. Successful mutating requests are appended to
// the journal before the response is returned; queries are never persisted.
func (p *Processor) Process(req map[string]any) map[string]any {
	resp := p.store.Handle(req)
	if ok, _ := resp["ok"].(bool); !ok {
		return resp
	}
	if op, _ := req["op"].(string); op == "query" {
		return resp
	}
	if err := p.enc.Encode(journalLine{Request: req}); err != nil {
		return errResp(fmt.Sprintf("persist state: %v", err))
	}
	if err := p.file.Sync(); err != nil {
		return errResp(fmt.Sprintf("sync state: %v", err))
	}
	return resp
}

// Close releases the journal file.
func (p *Processor) Close() error {
	return p.file.Close()
}
