package session

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/sloprail/harness-mocks/internal/compaction"
	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// UserInterrupted records, as a user message, what the agent is told of a turn the user
// interrupted (recorded: runs/interrupt-hook).
func (s *File) UserInterrupted() {
	s.User("<turn_aborted>\nThe user interrupted the previous turn on purpose. Any running unified exec processes may still be running in the background. If any tools/commands were aborted, they may have partially executed.\n</turn_aborted>")
}

// TurnAborted records that the turn was interrupted.
func (s *File) TurnAborted(turnID string) {
	s.append("event_msg", map[string]any{"type": "turn_aborted", "turn_id": turnID, "reason": "interrupted"})
}

// windows is how a session's compactions chain: each opens a window that names
// the one it closes, all of them naming the session's first.
type windows struct {
	first, previous string
	number          int
}

// userMessages are the user messages of the rollout so far, oldest first, with
// the ids they are kept under.
func (s *File) userMessages() (ids []string, text map[string]string) {
	text = map[string]string{}
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, text
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var rec struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) != nil || rec.Type != "response_item" ||
			rec.Payload.Type != "message" || rec.Payload.Role != "user" || len(rec.Payload.Content) == 0 {
			continue
		}
		id := "msg_" + coresession.NewID()
		ids = append(ids, id)
		text[id] = rec.Payload.Content[0].Text
	}
	return ids, text
}

// Turn is what the records a compaction leaves after it say of the session's turn and settings.
type Turn struct {
	SessionID, TurnID, Cwd, Model string
	// Sandbox is the sandbox mode of the run, empty when it asked for none (read-only, as Codex runs).
	Sandbox string
}

// Compacted records that the session was compacted, as Codex leaves it
// (recorded: runs/compaction-transcript-continuity). The rollout only grows: what came before
// (the user's messages, the tool calls and their outputs) stays in the file, and the same file, session
// and turn go on. Codex appends one `compacted` record that opens a new window naming the one it
// closes, and holds the history that replaces the conversation: the user's messages kept as they
// were, named by their retained source (and again in retained_context), and then the summary, an
// opaque item (Codex encrypts it); then the turn's context and the thread's settings again. A reader
// follows the chain across the compaction by the windows.
// sr:provides compaction-transcript-continuity/codex
func (s *File) Compacted(turn Turn) {
	if s.windows == nil {
		first := coresession.NewID()
		s.windows = &windows{first: first, previous: first}
	}
	ids, text := s.userMessages()
	plan := compaction.PlanBoundary(compaction.PlanInput{
		WithSegment: len(ids) > 0, Preserve: 1,
		Written:   func(n int) []string { return ids[max(0, len(ids)-n):] },
		LastUUID:  "",
		IsWritten: func(id string) bool { _, ok := text[id]; return ok },
		NewUUID:   coresession.NewID,
	})
	var history, meta, retained []map[string]any
	for i, id := range plan.Kept {
		history = append(history, map[string]any{"type": "message", "id": id, "role": "user",
			"content": []map[string]string{{"type": "input_text", "text": text[id]}}})
		revision := "retained_" + coresession.NewID()
		meta = append(meta, map[string]any{"client_authored": false, "user_input_order": i, "mcp_attribution": map[string]any{"status": "none"},
			"retained_source": map[string]any{"id": map[string]any{"message_id": id, "turn_id": turn.TurnID, "role": "user"}, "revision": revision, "complete": true}})
		retained = append(retained, map[string]any{"revision": revision, "order": i, "turn_id": turn.TurnID, "message_id": id, "text": text[id], "complete": true})
	}
	history = append(history, map[string]any{"type": "compaction", "id": "cmp_" + coresession.NewID(),
		"encrypted_content": "<opaque summary>"})
	meta = append(meta, map[string]any{"client_authored": false, "mcp_attribution": map[string]any{"status": "none"}})
	w := s.windows
	w.number++
	window := coresession.NewID()
	response := "resp_" + coresession.NewID()
	zero := map[string]any{"input_tokens": 0, "cached_input_tokens": 0, "cache_write_input_tokens": 0, "output_tokens": 0, "reasoning_output_tokens": 0, "total_tokens": 0}
	s.append("compacted", map[string]any{"message": "", "window_id": window, "previous_window_id": w.previous,
		"first_window_id": w.first, "window_number": w.number, "compaction_response_id": response,
		"replacement_history": history, "replacement_history_metadata": meta,
		"retained_context": map[string]any{"verified_answers": []any{}, "incomplete": false, "user_messages": retained, "user_messages_incomplete": false,
			"assistant_messages": []any{}, "assistant_messages_incomplete": false, "next_order": len(retained)},
		"resume_metadata": map[string]any{"multi_agent_version": "v1", "last_started_turn_id": turn.TurnID,
			"previous_turn_settings": map[string]any{"model": turn.Model, "realtime_active": false}},
		"latest_token_usage_record": map[string]any{"thread_id": turn.SessionID, "turn_id": turn.TurnID, "session_id": turn.SessionID, "root_turn_id": turn.TurnID,
			"response_id": response, "usage": zero, "turn_token_usage": zero, "thread_token_usage": zero}})
	w.previous = window
	s.afterCompaction(turn)
}
