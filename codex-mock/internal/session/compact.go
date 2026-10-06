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

// Compacted records that the session was compacted, as Codex leaves it
// (recorded: runs/compaction-transcript-continuity): one `compacted` record
// that opens a new window naming the one it closes, and holds the history that
// replaces the conversation: the user's first message kept as it was, named by
// its retained source, and then the summary, an opaque item (Codex encrypts
// it). A reader follows the chain across the compaction by the windows.
// sr:provides compaction-transcript-continuity/codex
func (s *File) Compacted() {
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
	var history, meta []map[string]any
	for _, id := range plan.Kept {
		history = append(history, map[string]any{"type": "message", "id": id, "role": "user",
			"content": []map[string]string{{"type": "input_text", "text": text[id]}}})
		meta = append(meta, map[string]any{"client_authored": false,
			"retained_source": map[string]any{"id": map[string]any{"message_id": id, "role": "user"}, "complete": true}})
	}
	history = append(history, map[string]any{"type": "compaction", "id": "cmp_" + coresession.NewID(),
		"encrypted_content": "<opaque summary>"})
	meta = append(meta, map[string]any{"client_authored": false})
	w := s.windows
	w.number++
	window := coresession.NewID()
	s.append("compacted", map[string]any{"message": "", "window_id": window, "previous_window_id": w.previous,
		"first_window_id": w.first, "window_number": w.number,
		"replacement_history": history, "replacement_history_metadata": meta})
	w.previous = window
}
