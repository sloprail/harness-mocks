package hooks

import (
	"context"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Command is one hook command to run.
type Command struct {
	// Line is the command, a shell line.
	Line string
	// Dir is the working directory it runs in.
	Dir string
	// Stdin is the event's payload, which the command reads.
	Stdin []byte
	// Env is the command's whole environment (procexec.Env).
	Env []string
	// Strict: the event fails on any non-zero exit status (VerdictOf).
	Strict bool
	// Timeout stops the command after this long; zero is no limit.
	Timeout time.Duration
}

// Run is how a hook command ended.
type Run struct {
	Stdout, Stderr string
	// ExitCode is the command's exit status; -1 when it did not exit by
	// itself (it could not start, or the timeout killed it).
	ExitCode int
	// Verdict is what the exit status decides on its own.
	Verdict Verdict
}

// Invoke runs a hook command through the shell, in its own process group, and
// reads its verdict off its exit status. A command that cannot start, or that
// is killed, has no exit status of its own: a non-blocking error.
func Invoke(ctx context.Context, c Command) Run {
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{"/bin/sh", "-c", c.Line}, Dir: c.Dir, Stdin: c.Stdin, Env: c.Env, Timeout: c.Timeout,
	})
	run := Run{Stdout: string(res.Stdout), Stderr: string(res.Stderr), ExitCode: res.ExitCode, Verdict: NonBlockingError}
	if err == nil && res.ExitCode >= 0 {
		run.Verdict = VerdictOf(res.ExitCode, c.Strict)
	}
	return run
}
