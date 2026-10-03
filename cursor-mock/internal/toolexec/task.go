package toolexec

import (
	"strings"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// taskRequired is what a Task call (Cursor's sub-agent dispatch) must carry:
// its prompt. Its description is not required: a call without one runs, with
// the description empty (recorded: runs/agent-input-validation and
// runs/agent-input-validation-description).
// sr:provides agent-input-validation/cursor
var taskRequired = subagents.PromptRequired

// InvalidArguments is what Cursor tells the agent of a Task call that lacks
// required parameters: "Invalid arguments:" and a line "<name>: Required" for
// each (recorded: runs/agent-input-validation). No hook fires for such a call.
func InvalidArguments(missing []string) string {
	lines := []string{"Invalid arguments:"}
	for _, p := range missing {
		lines = append(lines, p+": Required")
	}
	return strings.Join(lines, "\n")
}

// task is a Task call that passed validation. The mock runs no sub-agents, so
// it is answered as an error.
func task() Result {
	const msg = "sub-agents are not modelled by this mock"
	return Result{Failed: true, Frame: map[string]any{"error": map[string]any{"error": msg}}, ErrorMessage: msg}
}
