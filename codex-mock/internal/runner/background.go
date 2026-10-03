package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
	"github.com/sloprail/harness-mocks/internal/tools"
)

// yieldTime is how long a shell call waits for its command before it returns
// and leaves the command running in its own session (the call's
// `yield_time_ms`); zero when the call does not ask for it.
func yieldTime(c toolcall.Call) time.Duration {
	var in struct {
		Yield int `json:"yield_time_ms"`
	}
	_ = json.Unmarshal(c.Input, &in)
	return time.Duration(in.Yield) * time.Millisecond
}

// runYielding runs cmd for a call that yields: when the command ends within
// the yield time its result is the ordinary one; when it is still running the
// call returns what it printed so far and a session id, and the command goes
// on running, as a background task of the run (recorded:
// runs/bg-bash-reaped-at-exit).
func (h toolHost) runYielding(cmd string, yield time.Duration) (r tools.BashResult, running bool) {
	out, err := os.CreateTemp("", "codex-session-*")
	if err != nil {
		return tools.BashResult{Output: err.Error(), ExitCode: -1}, false
	}
	defer os.Remove(out.Name())
	t := tasks.NewTask(tasks.Command, fmt.Sprint(len(h.bg.Running())+1))
	if err := h.bg.StartCommand(t, tasks.CommandSpec{Argv: []string{"/bin/sh", "-c", cmd}, Dir: h.cfg.Cwd, Env: h.toolEnv, Out: out}); err != nil {
		return tools.BashResult{Output: err.Error(), ExitCode: -1}, false
	}
	select {
	case <-t.Done():
	case <-time.After(yield):
	}
	printed, _ := os.ReadFile(out.Name())
	if !t.Finished() {
		return tools.BashResult{Output: string(printed) + "\nsession_id=" + t.ID}, true
	}
	return tools.BashResult{Output: string(printed), ExitCode: t.ExitCode}, false
}

// reapAtExit ends the commands still running when the run's other work is
// done: `codex exec` exits at the end of its turn without waiting for them (no
// grace), and they are terminated.
// sr:provides background-bash-reaped-at-exit/codex
func (s *state) reapAtExit() {
	s.bg.ReapAtExit("", 0)
	s.bg.Shutdown()
}
