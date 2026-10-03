package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/interline-baggage-tracker/internal/baggage"
)

func main() {
	stateDir := flag.String("state", "", "directory holding journal.jsonl (created if missing)")
	flag.Parse()
	if *stateDir == "" {
		fmt.Fprintln(os.Stderr, "usage: baggage --state <state-dir>")
		os.Exit(2)
	}

	proc, err := baggage.OpenProcessor(*stateDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer proc.Close()

	if err := run(proc, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(proc *baggage.Processor, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(writer, map[string]any{"ok": false, "error": "invalid JSON: " + err.Error()})
			continue
		}
		writeResponse(writer, proc.Process(req))
	}
	return scanner.Err()
}

func writeResponse(w *bufio.Writer, resp map[string]any) {
	data, err := json.Marshal(resp)
	if err != nil {
		data = []byte(`{"ok":false,"error":"failed to encode response"}`)
	}
	if _, err := w.Write(append(data, '\n')); err == nil {
		w.Flush()
	}
}
