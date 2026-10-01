package subagents

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/hooks"
)

// DispatchRequired is what a call that dispatches a sub-agent must carry: a
// description of the task and the prompt the sub-agent starts from.
var DispatchRequired = []string{"description", "prompt"}

// MissingFromDispatch is the required parameters a dispatch's input lacks, in
// the order DispatchRequired gives them. A dispatch that lacks any is refused
// with the harness's input-validation error before any hook fires, and nothing
// runs.
//
// sr:capability agent-input-validation
func MissingFromDispatch(input json.RawMessage) []string {
	return hooks.RejectedInput(input, DispatchRequired)
}
