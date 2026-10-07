// Package session is the rollout file a codex-mock session leaves under
// $CODEX_HOME/sessions, which a scenario script reads as it goes.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// File is a session's append-only rollout: one JSON record per line.
type File struct {
	Path    string
	f       *os.File
	windows *windows
	next    int // the ordinal of the next line
}

// Create starts the rollout of session id at
// <home>/sessions/YYYY/MM/DD/rollout-<time>-<id>.jsonl, with its first record, the session's meta (see meta.go),
// keyed by the day and the session id, not by the working directory, and
// already there when the start hook runs (recorded: runs/session-transcript-file).
//
// sr:provides session-transcript-file/codex
func Create(home, id, cwd string, now time.Time) (*File, error) {
	return create(home, id, cwd, now, 0, nil)
}

// create starts the rollout with extra fields in its meta record; its first line has ordinal first.
func create(home, id, cwd string, now time.Time, first int, extra map[string]any) (*File, error) {
	dir := filepath.Join(home, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), id))
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	s := &File{Path: path, f: f, next: first}
	m := meta(id, cwd, now)
	for k, v := range extra {
		m[k] = v
	}
	s.append("session_meta", m)
	return s, nil
}

func (s *File) append(kind string, payload map[string]any) {
	enc := json.NewEncoder(s.f)
	enc.SetEscapeHTML(false)
	// Every line carries its position in the thread, which is what names a line that has no id of
	// its own (recorded: every run's rollout; a fork continues from the history it branched at).
	_ = enc.Encode(map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "ordinal": s.next, "type": kind, "payload": payload})
	s.next++
}

func message(role, ctype, text string) map[string]any {
	return map[string]any{"type": "message", "role": role,
		"content": []map[string]string{{"type": ctype, "text": text}}}
}

// User records a user message: the prompt, or the text a hook sent in its place
// (the reason a stop hook blocks with). It is all a hook leaves of its own:
// nothing records a hook that succeeded silently, failed or timed out.
// sr:provides hook-output-transcript-records/codex
func (s *File) User(text string) { s.append("response_item", message("user", "input_text", text)) }

// Developer records context a hook added.
// sr:provides hook-output-transcript-records/codex
func (s *File) Developer(text string) {
	s.append("response_item", message("developer", "input_text", text))
}

// Assistant records a message from the agent.
func (s *File) Assistant(text string) {
	s.append("response_item", message("assistant", "output_text", text))
}

// CodeCall records a tool call the agent made, as Codex records every one: a call of its code-mode
// `exec` tool, whose input is the JS that calls the tool (recorded in every run's rollout: a
// custom_tool_call named exec, e.g. `const r = await tools.exec_command({cmd:"ls"}); text(r.output);`).
func (s *File) CodeCall(callID, js string) {
	sum := sha256.Sum256([]byte(callID))
	s.append("response_item", map[string]any{"type": "custom_tool_call", "id": "ctc_" + hex.EncodeToString(sum[:12]),
		"status": "completed", "call_id": callID, "name": "exec", "input": js})
}

// ToolOutput records what the agent was told a tool call returned.
func (s *File) ToolOutput(callID, output string) {
	s.append("response_item", map[string]any{"type": "function_call_output", "call_id": callID, "output": output})
}

// ToolOutputParts records a tool result that the harness tells the agent as parts, one after another.
func (s *File) ToolOutputParts(callID string, parts ...string) {
	out := make([]map[string]string, len(parts))
	for i, p := range parts {
		out[i] = map[string]string{"type": "input_text", "text": p}
	}
	s.append("response_item", map[string]any{"type": "function_call_output", "call_id": callID, "output": out})
}

// Close ends the rollout.
func (s *File) Close() error { return s.f.Close() }
