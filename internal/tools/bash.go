// Package tools is the harness-neutral core of the built-in tools a mock runs
// itself.
package tools

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// BashResult is what a shell command left.
type BashResult struct {
	// Output is its stdout followed by its stderr.
	Output string
	// ExitCode is its exit status; -1 when it did not exit by itself (the
	// shell could not start, or the command was killed).
	ExitCode int
}

// Failed reports whether the command ended with a non-zero status.
func (r BashResult) Failed() bool { return r.ExitCode != 0 }

// Bash runs command with /bin/sh in dir, with env as its whole environment
// (procexec.Env builds it), and waits for it. A shell that cannot start is
// reported as the output of an exit status of -1.
func Bash(ctx context.Context, command, dir string, env []string) BashResult {
	res, err := procexec.Run(ctx, procexec.Spec{Argv: []string{"/bin/sh", "-c", command}, Dir: dir, Env: env})
	if err != nil {
		return BashResult{Output: err.Error(), ExitCode: -1}
	}
	return BashResult{Output: string(res.Stdout) + string(res.Stderr), ExitCode: res.ExitCode}
}
