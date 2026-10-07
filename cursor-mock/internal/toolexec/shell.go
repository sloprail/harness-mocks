package toolexec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// Shell is the shell a Shell call's command line runs in, the same on every OS:
// cursor-agent runs the user's shell (bash or zsh, never POSIX sh), and recorded
// runs/shell-compound-more has `[[ ]]` and `<<<` working. /bin/sh is dash on Linux
// and bash on macOS, so it is named explicitly.
const Shell = "/bin/bash"

// shell runs a Shell call. A command exiting non-zero failed, whatever its
// status (grep finding nothing is a failure too): the result is a failure
// carrying the exit code and both streams, and the failure hook's
// error_message is what the command printed, or else "Command failed with
// exit code N" (recorded: runs/tool-failure, runs/shell-exit-status).
//
// sr:provides bash-tool-result/cursor
// sr:docs https://cursor.com/docs/hooks#aftershellexecution
func shell(ctx context.Context, c Call, dir string, env []string) Result {
	if r, refused := RefusesRipgrep(c); refused {
		return r
	}
	cwd := dir
	if wd := c.str("workingDirectory"); wd != "" {
		cwd = wd
	}
	start := time.Now()
	res := tools.BashArgv(ctx, []string{Shell, "-c", c.Command()}, cwd, env)
	took := time.Since(start)
	body := map[string]any{
		"command": c.Command(), "workingDirectory": c.str("workingDirectory"), "exitCode": res.ExitCode, "signal": "",
		"stdout": res.Stdout, "stderr": res.Stderr, "executionTime": max(took.Milliseconds(), 1), "localExecutionTimeMs": max(took.Milliseconds(), 1), "interleavedOutput": res.Output,
	}
	r := Result{Output: res.Output, Took: took, ToolOutput: jsonString(struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exitCode"`
	}{res.Output, res.ExitCode})}
	if !res.Failed() {
		r.Frame = map[string]any{"success": body, "isBackground": false}
		return r
	}
	body["aborted"] = false
	r.Failed, r.Frame = true, map[string]any{"failure": body, "isBackground": false}
	r.ErrorMessage = strings.TrimSpace(res.Output)
	if r.ErrorMessage == "" {
		r.ErrorMessage = fmt.Sprintf("Command failed with exit code %d", res.ExitCode)
	}
	return r
}

// RefusesRipgrep refuses a command that names the harness's ripgrep: the
// shell sees CURSOR_RIPGREP_PATH as recorded, but the mock has no ripgrep at
// that path, so a command that uses it is not run (fail fast, not a missing
// binary run silently).
func RefusesRipgrep(c Call) (Result, bool) {
	cmd := c.Command()
	if !strings.Contains(cmd, "CURSOR_RIPGREP_PATH") && !strings.Contains(cmd, "/.local/share/cursor-agent/") {
		return Result{}, false
	}
	msg := NotModeledPrefix + "the harness's ripgrep (CURSOR_RIPGREP_PATH) is not modeled: a command that uses it is refused"
	return failed(msg, msg), true
}
