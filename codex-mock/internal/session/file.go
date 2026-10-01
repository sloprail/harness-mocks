// Package session is the rollout file a codex-mock session leaves under
// $CODEX_HOME/sessions, which a scenario script reads as it goes.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// File is a session's append-only rollout: one JSON record per line.
type File struct {
	Path string
	f    *os.File
}

// Create starts the rollout of session id at
// <home>/sessions/YYYY/MM/DD/rollout-<time>-<id>.jsonl, with its meta record.
func Create(home, id, cwd string, now time.Time) (*File, error) {
	dir := filepath.Join(home, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), id))
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	s := &File{Path: path, f: f}
	s.append("session_meta", map[string]any{"id": id, "session_id": id, "cwd": cwd,
		"originator": "codex_exec", "source": "exec", "cli_version": "mock"})
	return s, nil
}

func (s *File) append(kind string, payload map[string]any) {
	enc := json.NewEncoder(s.f)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "type": kind, "payload": payload})
}

func message(role, ctype, text string) map[string]any {
	return map[string]any{"type": "message", "role": role,
		"content": []map[string]string{{"type": ctype, "text": text}}}
}

// User records a user message: the prompt, or the text a hook sent in its place.
func (s *File) User(text string) { s.append("response_item", message("user", "input_text", text)) }

// Developer records context a hook added.
func (s *File) Developer(text string) {
	s.append("response_item", message("developer", "input_text", text))
}

// Assistant records a message from the agent.
func (s *File) Assistant(text string) {
	s.append("response_item", message("assistant", "output_text", text))
}

// ToolCall records a tool call the agent made.
func (s *File) ToolCall(callID, name string, input json.RawMessage) {
	s.append("response_item", map[string]any{"type": "function_call", "call_id": callID, "name": name, "arguments": string(input)})
}

// ToolOutput records what the agent was told a tool call returned.
func (s *File) ToolOutput(callID, output string) {
	s.append("response_item", map[string]any{"type": "function_call_output", "call_id": callID, "output": output})
}

// Close ends the rollout.
func (s *File) Close() error { return s.f.Close() }
