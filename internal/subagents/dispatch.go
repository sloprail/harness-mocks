package subagents

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/hooks"
)

// DispatchRequired is what a call that dispatches a sub-agent must carry: a
// description of the task and the prompt the sub-agent starts from.
var DispatchRequired = []string{"description", "prompt"}

// What a call that dispatches a sub-agent must carry is the harness's to say;
// these are the parameter sets a harness names its own with (Cursor's Task
// needs its prompt only, Codex's spawn_agent a message).
var (
	PromptRequired  = []string{"prompt"}
	MessageRequired = []string{"message"}
)

// MissingFromDispatch is the required parameters a dispatch's input lacks, in
// the order DispatchRequired gives them. A dispatch that lacks any is refused
// with the harness's input-validation error before any hook fires, and nothing
// runs.
//
// sr:capability agent-input-validation
func MissingFromDispatch(input json.RawMessage) []string {
	return hooks.RejectedInput(input, DispatchRequired)
}
