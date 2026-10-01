// Package tools is the harness-neutral core of executing the built-in tools.
package tools

import (
	"context"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Shell is a shell command to run for a tool.
type Shell struct {
	// Command is the shell line, run with /bin/sh -c.
	Command string
	// Dir is the working directory.
	Dir string
	// Env is the command's whole environment (procexec.Env).
	Env []string
	// Timeout stops the command after this long; zero is no limit.
	Timeout time.Duration
}

// ShellResult is how a shell command ended.
type ShellResult struct {
	Stdout, Stderr string
	// ExitCode is the exit status; -1 when the command did not exit by itself.
	ExitCode int
	// Took is how long it ran.
	Took time.Duration
}

// Output is what the command printed: its stdout, then its stderr.
func (r ShellResult) Output() string { return r.Stdout + r.Stderr }

// Failed reports whether the command failed: it did not exit with status 0.
func (r ShellResult) Failed() bool { return r.ExitCode != 0 }

// RunShell runs a shell command for a tool, in its own process group. A shell
// that cannot start is an error; a command exiting non-zero is a result.
func RunShell(ctx context.Context, s Shell) (ShellResult, error) {
	start := time.Now()
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{"/bin/sh", "-c", s.Command}, Dir: s.Dir, Env: s.Env, Timeout: s.Timeout,
	})
	return ShellResult{Stdout: string(res.Stdout), Stderr: string(res.Stderr), ExitCode: res.ExitCode, Took: time.Since(start)}, err
}
