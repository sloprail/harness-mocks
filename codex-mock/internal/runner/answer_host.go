package runner

import (
	"fmt"

	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// Answer records what the agent was told, in Codex's words.
func (h toolHost) Answer(c toolcall.Call, a toolcall.Answer) {
	var text string
	switch a.Kind {
	case toolcall.Unknown:
		text = "unsupported call: " + c.Name
	case toolcall.Invalid:
		if c.Name == agentTool {
			text = spawnRefusal
			break
		}
		text = fmt.Sprintf("failed to parse function arguments: missing field `%s`", a.Missing[0])
	case toolcall.Refused:
		text = fmt.Sprintf("Command blocked by PreToolUse hook: %s. Command: %s", a.Reason, command(c))
		fmt.Fprintf(h.cfg.Stderr, "ERROR codex_core::tools::router: error=%s\n", text)
	case toolcall.Done:
		text = a.Result.Output
		if c.Name == patchTool && !a.Result.Failed {
			text = patchTold
		}
		if a.Replaced {
			text = a.Feedback
			fmt.Fprintf(h.cfg.Stderr, "ERROR codex_core::tools::router: error=%s\n", text)
		}
	}
	if a.Kind == toolcall.Done && a.Result.Ended && !a.Replaced {
		h.rollout.ToolOutputParts(c.ID, completedFrame(a.Result.Wall), text) // as the harness tells the agent a command that ran to its end
	} else {
		h.rollout.ToolOutput(c.ID, text)
	}
	h.prog.Move(0, 1) // finished: what another agent's gate may wait for
	if c.Name == agentTool && a.Kind == toolcall.Done && !a.Replaced && !a.Result.Failed {
		h.startBackground(c, a.Result.Output) // a dispatch not waited for runs once it is answered
	}
}
