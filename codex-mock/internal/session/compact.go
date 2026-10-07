package session

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

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

// afterCompaction writes what Codex writes after the compacted record: the turn's context again, and
// the thread's settings as applied.
func (s *File) afterCompaction(turn Turn) {
	profile, policy := sandboxRecords(turn.Cwd, turn.Sandbox)
	collab := map[string]any{"mode": "default", "settings": map[string]any{"model": turn.Model, "reasoning_effort": nil, "developer_instructions": nil}}
	zone, _ := time.Now().Zone()
	s.append("turn_context", map[string]any{"turn_id": turn.TurnID, "root_turn_id": turn.TurnID, "disabled_plugin_ids": []any{}, "cwd": turn.Cwd,
		"workspace_roots": []string{turn.Cwd}, "current_date": time.Now().Format("2006-01-02"), "timezone": zone, "approval_policy": "never",
		"approvals_reviewer": "user", "sandbox_policy": policy, "permission_profile": profile, "model": turn.Model, "collaboration_mode": collab,
		"multi_agent_version": "v1", "realtime_active": false, "effort": nil, "summary": "none"})
	s.append("event_msg", map[string]any{"type": "thread_settings_applied", "thread_id": turn.SessionID, "thread_settings": map[string]any{
		"model": turn.Model, "model_provider_id": "openai", "approval_policy": "never", "approvals_reviewer": "user", "permission_profile": profile,
		"cwd": turn.Cwd, "runtime_workspace_roots": []string{turn.Cwd}, "reasoning_effort": nil, "collaboration_mode": collab, "disabled_plugin_ids": []any{}}})
}

// sandboxRecords are the permission profile and the sandbox policy Codex records for a sandbox mode
// (recorded: the turn_context of runs with each mode; none asked for is read-only).
func sandboxRecords(cwd, mode string) (profile, policy map[string]any) {
	switch mode {
	case "danger-full-access":
		return map[string]any{"type": "disabled"}, map[string]any{"type": "danger-full-access"}
	case "workspace-write":
		path := func(p, access string) map[string]any {
			return map[string]any{"path": map[string]any{"type": "path", "path": p}, "access": access, "missing_path_behavior": "skip"}
		}
		special := func(kind, access string) map[string]any {
			return map[string]any{"path": map[string]any{"type": "special", "value": map[string]any{"kind": kind}}, "access": access}
		}
		entries := []any{special("root", "read"), map[string]any{"path": map[string]any{"type": "path", "path": cwd}, "access": "write"},
			special("slash_tmp", "write"), special("tmpdir", "write"),
			path(cwd+"/.git", "read"), path(cwd+"/.agents", "read"), path(cwd+"/.codex", "read"), path(cwd+"/.aws", "read")}
		return map[string]any{"type": "managed", "file_system": map[string]any{"type": "restricted", "entries": entries}, "network": "restricted"},
			map[string]any{"type": "workspace-write", "network_access": false, "exclude_tmpdir_env_var": false, "exclude_slash_tmp": false}
	}
	return map[string]any{"type": "managed", "file_system": map[string]any{"type": "restricted", "entries": []any{
			map[string]any{"path": map[string]any{"type": "special", "value": map[string]any{"kind": "root"}}, "access": "read"}}}, "network": "restricted"},
		map[string]any{"type": "read-only"}
}
