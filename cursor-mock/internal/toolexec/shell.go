package toolexec

import (
	"context"
	"fmt"
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/tools"
)

// shell runs a Shell call. A command exiting non-zero failed: the failure
// hook's error_message is what it wrote on stderr, or else "Command failed
// with exit code N" (recorded: runs/tool-failure).
func shell(ctx context.Context, c Call, dir string, env []string) Result {
	cwd := dir
	if wd := c.str("workingDirectory"); wd != "" {
		cwd = wd
	}
	res, err := tools.RunShell(ctx, tools.Shell{Command: c.Command(), Dir: cwd, Env: env})
	out := res.Output()
	body := map[string]any{
		"command": c.Command(), "workingDirectory": c.str("workingDirectory"), "exitCode": res.ExitCode, "signal": "",
		"stdout": res.Stdout, "stderr": res.Stderr, "executionTime": res.Took.Milliseconds(), "interleavedOutput": out,
	}
	r := Result{Output: out, Took: res.Took, ToolOutput: jsonString(struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exitCode"`
	}{out, res.ExitCode})}
	if err == nil && !res.Failed() {
		r.Outcome, r.Frame = corehooks.ToolSucceeded, map[string]any{"success": body, "isBackground": false}
		return r
	}
	body["aborted"] = false
	r.Outcome, r.Frame = corehooks.ToolFailed, map[string]any{"failure": body, "isBackground": false}
	r.ErrorMessage = strings.TrimSpace(res.Stderr)
	if r.ErrorMessage == "" {
		r.ErrorMessage = fmt.Sprintf("Command failed with exit code %d", res.ExitCode)
	}
	return r
}
