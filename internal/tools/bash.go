// Package tools is the harness-neutral core of the built-in tools a mock runs
// itself.
package tools

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// BashResult is what a shell command left.
type BashResult struct {
	// Stdout and Stderr are what it printed on each.
	Stdout, Stderr string
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
	return BashArgv(ctx, []string{"/bin/sh", "-c", command}, dir, env)
}

// BashDetached is Bash for a harness whose shell lets a background job (`cmd &`) outlive the
// command that started it.
func BashDetached(ctx context.Context, command, dir string, env []string) BashResult {
	return bashRun(ctx, procexec.Spec{Argv: []string{"/bin/sh", "-c", command}, Dir: dir, Env: env, LeaveGroup: true})
}

// BashArgv is Bash with the command line a harness runs it by (a named shell
// and its flags): the same, but for the argv.
func BashArgv(ctx context.Context, argv []string, dir string, env []string) BashResult {
	return bashRun(ctx, procexec.Spec{Argv: argv, Dir: dir, Env: env})
}

func bashRun(ctx context.Context, spec procexec.Spec) BashResult {
	res, err := procexec.Run(ctx, spec)
	if err != nil {
		return BashResult{Output: err.Error(), ExitCode: -1}
	}
	return BashResult{Stdout: string(res.Stdout), Stderr: string(res.Stderr), Output: string(res.Stdout) + string(res.Stderr), ExitCode: res.ExitCode}
}
