package toolexec

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	start := time.Now()
	res := tools.Bash(ctx, c.Command(), cwd, env)
	took := time.Since(start)
	body := map[string]any{
		"command": c.Command(), "workingDirectory": c.str("workingDirectory"), "exitCode": res.ExitCode, "signal": "",
		"stdout": res.Stdout, "stderr": res.Stderr, "executionTime": took.Milliseconds(), "interleavedOutput": res.Output,
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
	r.ErrorMessage = strings.TrimSpace(res.Stderr)
	if r.ErrorMessage == "" {
		r.ErrorMessage = fmt.Sprintf("Command failed with exit code %d", res.ExitCode)
	}
	return r
}
