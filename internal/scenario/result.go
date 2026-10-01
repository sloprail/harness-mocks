// Package scenario is the harness-neutral core of running a scripted agent to
// completion: its records and its one result.
package scenario

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RecordType is the type of a JSONL record line, or an error when the line is
// not a JSON object, has no type, or has one outside known (matched without
// regard to case).
func RecordType(line []byte, known map[string]bool) (string, error) {
	var rec struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &rec); err != nil {
		return "", fmt.Errorf("not valid JSON: %w", err)
	}
	if rec.Type == "" {
		return "", fmt.Errorf("missing required field \"type\"")
	}
	if !known[strings.ToLower(rec.Type)] {
		return "", fmt.Errorf("unknown record type %q", rec.Type)
	}
	return rec.Type, nil
}

// Result is a run's result frame: held when a turn ends, because the turn may
// still go on.
type Result struct {
	line []byte
}

// Hold keeps line as the run's result, replacing any held before it: a turn
// that goes on ends with the result of its real end.
func (r *Result) Hold(line []byte) { r.line = line }

// Continue drops the held result: the turn went on.
func (r *Result) Continue() { r.line = nil }

// Finish hands the held result to write exactly once, ending the run, and
// reports whether there was one.
//
// sr:capability noninteractive-run
func (r *Result) Finish(write func(line []byte)) bool {
	if r.line == nil {
		return false
	}
	line := r.line
	r.line = nil
	write(line)
	return true
}
