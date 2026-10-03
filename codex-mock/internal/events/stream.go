// Package events is the JSONL event stream `codex exec --json` prints.
package events

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Stream writes the events of one run, one JSON object per line.
type Stream struct {
	out  io.Writer
	next int
}

// New is a stream on out.
func New(out io.Writer) *Stream { return &Stream{out: out} }

func (s *Stream) emit(v map[string]any) {
	enc := json.NewEncoder(s.out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (s *Stream) newID() string {
	id := fmt.Sprintf("item_%d", s.next)
	s.next++
	return id
}

// ThreadStarted opens the run: the thread is the session.
func (s *Stream) ThreadStarted(id string) {
	s.emit(map[string]any{"type": "thread.started", "thread_id": id})
}

// TurnStarted opens a turn: from the user's prompt to the final message.
func (s *Stream) TurnStarted() { s.emit(map[string]any{"type": "turn.started"}) }

// TurnCompleted closes the turn. The mock calls no model, so it used no tokens.
func (s *Stream) TurnCompleted() {
	s.emit(map[string]any{"type": "turn.completed", "usage": map[string]int{
		"input_tokens": 0, "cached_input_tokens": 0, "cache_write_input_tokens": 0,
		"output_tokens": 0, "reasoning_output_tokens": 0}})
}

// AgentMessage reports a message from the agent.
func (s *Stream) AgentMessage(text string) {
	s.emit(map[string]any{"type": "item.completed", "item": map[string]any{
		"id": s.newID(), "type": "agent_message", "text": text}})
}

// ShellCommand is how Codex shows a command the shell ran.
func ShellCommand(command string) string {
	return "/bin/sh -c '" + strings.ReplaceAll(command, "'", `'\''`) + "'"
}

// CommandStarted reports a command that began; the id pairs it with its end.
func (s *Stream) CommandStarted(command string) string {
	id := s.newID()
	s.emit(map[string]any{"type": "item.started", "item": map[string]any{
		"id": id, "type": "command_execution", "command": ShellCommand(command),
		"aggregated_output": "", "exit_code": nil, "status": "in_progress"}})
	return id
}

// CommandCompleted reports how a started command ended: its exit status and
// everything it printed, and a status of completed on exit 0, failed otherwise
// (recorded: runs/shell-exit-status). The agent is told the output as it is,
// failed or not; the failure shows only here.
// sr:provides bash-tool-result/codex
func (s *Stream) CommandCompleted(id, command, output string, exit int) {
	status := "completed"
	if exit != 0 {
		status = "failed"
	}
	s.emit(map[string]any{"type": "item.completed", "item": map[string]any{
		"id": id, "type": "command_execution", "command": ShellCommand(command),
		"aggregated_output": output, "exit_code": exit, "status": status}})
}
