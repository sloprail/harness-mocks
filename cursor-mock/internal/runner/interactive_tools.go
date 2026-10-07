package runner

import (
	"fmt"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// refusesTUITool refuses the run when a tool call in a TUI session is not modeled (tuiToolRefusal),
// and says whether it did.
func (s *session) refusesTUITool(tu scenario.ToolUse) bool {
	if !s.cfg.Interactive {
		return false
	}
	msg := s.tuiToolRefusal(tu)
	if msg != "" {
		s.refusal.refuse(msg)
	}
	return msg != ""
}

// tuiToolRefusal is why a tool call is not modeled in a TUI session, "" when it is: a shell
// command, a file write or edit and a file read, run with --force (recorded: runs/tui-tools; the
// TUI asks before it runs a command without it, and what it asks was not recorded). Everything
// else the print mode models (a command left in the background, a search, a sub-agent, an MCP
// tool) was not recorded in the TUI.
func (s *session) tuiToolRefusal(tu scenario.ToolUse) string {
	if !s.cfg.Force {
		return "cursor-mock: a tool call in a TUI session is modeled only with --force (the mode its recordings cover): without it the TUI asks before it runs one"
	}
	c := toolexec.FromScript(tu.Name, tu.Input)
	switch {
	case c.Kind == "shellToolCall" && !c.Background(), c.Kind == "editToolCall", c.Kind == "readToolCall":
		return ""
	}
	return fmt.Sprintf("cursor-mock: the %s tool call (as a %s) in a TUI session is not modeled: only Shell commands that finish, Write, Edit and Read were recorded (runs/tui-tools)", tu.Name, c.Kind)
}

// withUsage adds what the payloads of the end of a response carry of the model's usage: counts
// the real service measures, which the mock has no model to give, so they are fixed.
func (s *session) withUsage(p map[string]any) map[string]any {
	p["input_tokens"], p["output_tokens"], p["cache_read_tokens"], p["cache_write_tokens"] = 1000, 10, 500, 0
	return p
}
