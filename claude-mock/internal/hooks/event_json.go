package hooks

import (
	"bytes"
	"encoding/json"
)

// MarshalJSON writes the payload, with agent_type present (possibly "")
// whenever agent_id is: real Claude Code sends both together — a manual
// compaction's summarizer fires SubagentStop with agent_type "" (claude
// 2.1.282).
func (in Input) MarshalJSON() ([]byte, error) {
	type plain Input
	b, err := json.Marshal(plain(in))
	if err != nil || in.AgentID == "" || in.AgentType != "" {
		return b, err
	}
	id, err := json.Marshal(in.AgentID)
	if err != nil {
		return nil, err
	}
	// agent_type "" follows agent_id, the first place the id occurs (it is a top-level key written before any tool input)
	at := []byte(`"agent_id":` + string(id))
	n := bytes.Index(b, at) + len(at)
	return append(b[:n:n], append([]byte(`,"agent_type":""`), b[n:]...)...), nil
}

// strictExitEvents are Claude Code's events that fail on any non-zero exit,
// not only exit 2 (docs: "Any non-zero exit code causes worktree creation to
// fail"; the same for removal).
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
var strictExitEvents = map[EventName]bool{EventWorktreeCreate: true, EventWorktreeRemove: true}
