package hooks

import "sync"

// Turn is the user prompt a session is on. A session has one, shared by every
// agent inside it (a sub-agent's events carry the session's prompt); it has
// none until the first prompt starts.
type Turn struct {
	mu sync.Mutex
	id string
}

// Current is the id of the prompt the session is on; empty before the first.
func (t *Turn) Current() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.id
}

// begin starts a prompt with a fresh id and returns it.
func (t *Turn) begin(newID func() string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.id = newID()
	return t.id
}

// Ensure is the current prompt's id, a new prompt's when the session is on none:
// a compaction run on a resumed session acts as the prompt that has no
// UserPromptSubmit (recorded: claude snapshots/runs/compact).
func (t *Turn) Ensure(newID func() string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.id == "" {
		t.id = newID()
	}
	return t.id
}

// PromptEvent is how an event relates to the prompt the session is on.
type PromptEvent int

const (
	// PromptBegins: the user's prompt starts a new one.
	PromptBegins PromptEvent = iota
	// PromptContinues: an event about a turn of the conversation, within the
	// current prompt (a tool call, the agent stopping, a task notification
	// handed to the model as a prompt).
	PromptContinues
	// PromptSurrounds: an event of the session's or a sub-agent's own life or of
	// a compaction, which carries the current prompt but is not about a turn.
	PromptSurrounds
)

// PromptFields completes a payload's prompt id and permission mode: a
// beginning prompt gets a fresh id, every other event the current prompt's
// (none before the first); the permission mode is told only on the events about
// a turn (PromptBegins, PromptContinues), not on the ones that surround it.
func PromptFields(t *Turn, newID func() string, e PromptEvent, mode string) (promptID, permissionMode string) {
	if e == PromptBegins {
		promptID = t.begin(newID)
	} else {
		promptID = t.Current()
	}
	if e != PromptSurrounds {
		permissionMode = mode
	}
	return promptID, permissionMode
}
