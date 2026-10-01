// Package toolexec runs the tool calls codex-mock executes itself.
package toolexec

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Result is what a shell command left.
type Result struct {
	// Output is its stdout then its stderr, what Codex reports as the
	// command's aggregated output.
	Output string
	// ExitCode is its exit status; -1 when the shell could not start.
	ExitCode int
}

// Bash runs a command with /bin/sh in dir, in env.
func Bash(ctx context.Context, command, dir string, env []string) Result {
	res, err := procexec.Run(ctx, procexec.Spec{Argv: []string{"/bin/sh", "-c", command}, Dir: dir, Env: env})
	if err != nil {
		return Result{Output: err.Error(), ExitCode: -1}
	}
	return Result{Output: string(res.Stdout) + string(res.Stderr), ExitCode: res.ExitCode}
}
