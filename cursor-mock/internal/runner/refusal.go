package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// refusal is the first refusal of something the mock does not model that a run
// met: a tool called in a way no recording shows, a flag, a model. It fails the
// run, so that a refusal inside a sub-agent, whose frames are not printed, cannot
// pass unseen. The mock's own refusals are worded "cursor-mock: ...".
type refusal struct {
	mu     sync.Mutex
	msg    string
	cancel context.CancelFunc
}

// refuse records the refusal and stops the run.
func (r *refusal) refuse(msg string) {
	r.mu.Lock()
	if r.msg == "" {
		r.msg = msg
	}
	r.mu.Unlock()
	r.cancel()
}

// refuseResult refuses the run when a tool's result is the mock's refusal.
func (s *session) refuseResult(r toolexec.Result) {
	if msg, ok := r.NotModeled(); ok {
		s.refusal.refuse(msg)
	}
}

// refuseMsg refuses the run when msg is the mock's refusal, and returns it.
func (s *session) refuseMsg(msg string) string {
	if strings.HasPrefix(msg, toolexec.NotModeledPrefix) {
		s.refusal.refuse(msg)
	}
	return msg
}

func (r *refusal) message() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.msg
}

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
