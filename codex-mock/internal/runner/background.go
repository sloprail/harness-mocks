package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
	"github.com/sloprail/harness-mocks/internal/tools"
)

// yieldTime is how long a shell call waits for its command before it returns
// and leaves the command running in its own session (the call's
// `yield_time_ms`), and whether the call asks for it at all. A yield time of
// zero returns at once, with the receipt, whatever the command does
// (recorded: runs/task-notifications-bg).
func yieldTime(c toolcall.Call) (time.Duration, bool) {
	var in struct {
		Yield *int `json:"yield_time_ms"`
	}
	_ = json.Unmarshal(c.Input, &in)
	if in.Yield == nil {
		return 0, false
	}
	return time.Duration(*in.Yield) * time.Millisecond, true
}

// runYielding runs argv (the command by its shell) for call c that yields: when the command ends within
// the yield time its result is the ordinary one; when it is still running the
// call returns a receipt, {chunk_id, wall_time_seconds, session_id,
// original_token_count, output}: the session id of the command and what it has
// printed so far, and no file its output goes to. The command goes on running
// as a background task of the run, tied to the call (task Meta), and the call
// fires no PostToolUse while it does (recorded: runs/background-bash-start).
//
// sr:docs https://developers.openai.com/codex/hooks#tool-coverage
func (h toolHost) runYielding(ctx context.Context, c toolcall.Call, item string, argv []string, yield time.Duration) (r tools.BashResult, running bool) {
	start := time.Now()
	out, err := os.CreateTemp("", "codex-session-*")
	if err != nil {
		return tools.BashResult{Output: err.Error(), ExitCode: -1}, false
	}
	sid := 10000 + rand.Intn(90000)
	t := tasks.NewTask(tasks.Command, strconv.Itoa(sid))
	t.Meta = c.ID
	ended, err := h.bg.StartYielding(ctx, t, tasks.CommandSpec{Argv: argv, Dir: h.cfg.Cwd, Env: h.toolEnv, Out: out}, yield)
	if err != nil {
		return tools.BashResult{Output: err.Error(), ExitCode: -1}, false
	}
	printed, _ := os.ReadFile(out.Name())
	if !ended {
		h.sessions.add(sid, &liveCommand{call: c, item: item, cmd: command(c), out: out.Name(), read: int64(len(printed))})
		receipt, _ := json.Marshal(map[string]any{
			"chunk_id": fmt.Sprintf("%06x", rand.Intn(1<<24)), "wall_time_seconds": time.Since(start).Seconds(),
			"session_id": sid, "original_token_count": len(printed) / 4, "output": string(printed)})
		return tools.BashResult{Output: string(receipt)}, true
	}
	os.Remove(out.Name())
	return tools.BashResult{Output: string(printed), ExitCode: t.ExitCode}, false
}

// stillRunning reports whether call id has a command left running.
func (h toolHost) stillRunning(id string) bool {
	for _, t := range h.bg.Running() {
		if t.Meta == id {
			return true
		}
	}
	return false
}

// reapAtExit ends the commands still running when the run's other work is
// done: `codex exec` exits at the end of its turn without waiting for them (no
// grace), and they are terminated.
// sr:provides background-bash-reaped-at-exit/codex
func (s *state) reapAtExit() {
	s.bg.ReapAtExit("", 0)
	s.bg.Shutdown()
}
