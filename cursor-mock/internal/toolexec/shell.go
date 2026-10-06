package toolexec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// shell runs a Shell call. A command exiting non-zero failed, whatever its
// status (grep finding nothing is a failure too): the result is a failure
// carrying the exit code and both streams, and the failure hook's
// error_message is what the command printed, or else "Command failed with
// exit code N" (recorded: runs/tool-failure, runs/shell-exit-status).
//
// sr:provides bash-tool-result/cursor
// sr:docs https://cursor.com/docs/hooks#aftershellexecution
func shell(ctx context.Context, c Call, dir string, env []string) Result {
	cwd := dir
	if wd := c.str("workingDirectory"); wd != "" {
		cwd = wd
	}
	start := time.Now()
	res := tools.Bash(ctx, c.Command(), cwd, env)
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
