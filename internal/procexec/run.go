package procexec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Spec is one child process to run to completion.
type Spec struct {
	// Argv is the program and its arguments.
	Argv []string
	// Dir is the working directory; empty is the mock's own.
	Dir string
	// Stdin is what the child reads; nil is an empty input.
	Stdin []byte
	// Env is the child's whole environment: build it with Env.
	Env []string
	// Timeout stops the child after this long; zero is no limit.
	Timeout time.Duration
	// Stderr, when set, receives the child's stderr as well as Result.Stderr.
	Stderr io.Writer
	// Stdout, when set, receives the child's stdout as well as Result.Stdout, as it is written.
	Stdout io.Writer
	// OnStart, when set, is given, once the child has started, a function that sends it a signal
	// (a user's Ctrl-C, say, when its output shows what the test waits for).
	OnStart func(signal func(os.Signal) error)
	// NewSession runs the child in a session of its own, so it has no
	// controlling terminal; its process group is still its own.
	NewSession bool
	// LeaveGroup lets what the child started in the background (a shell `&` job) outlive it:
	// the group is killed when ctx or Timeout ends the run, not when the child exits, and not
	// when the mock exits (adr/child-processes: claude's Bash tool).
	LeaveGroup bool
}

// Result is how a child ended.
type Result struct {
	// Stdout and Stderr are everything the child wrote.
	Stdout, Stderr []byte
	// ExitCode is the child's exit status; -1 when it did not exit by itself
	// (started and was killed, or did not start).
	ExitCode int
	// Started is whether the program could be started at all.
	Started bool
	// TimedOut is whether Timeout stopped it.
	TimedOut bool
}

// Run starts the child in its own process group, waits for it and returns how
// it ended. Cancelling ctx, or the timeout, kills the whole group, so no
// grandchild outlives the mock. An error is returned only when the child
// cannot be started; a child that exits non-zero is a Result.
func Run(ctx context.Context, s Spec) (Result, error) {
	if len(s.Argv) == 0 {
		return Result{ExitCode: -1}, errors.New("procexec: no program to run")
	}
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, s.Argv[0], s.Argv[1:]...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if s.NewSession { // a session leader is also a group leader
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	if s.Stdin != nil {
		cmd.Stdin = bytes.NewReader(s.Stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	if s.Stdout != nil {
		cmd.Stdout = io.MultiWriter(&out, s.Stdout)
	}
	cmd.Stderr = &errb
	if s.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&errb, s.Stderr)
	}
	res := Result{ExitCode: -1}
	if err := cmd.Start(); err != nil {
		return res, err
	}
	res.Started = true
	if s.OnStart != nil {
		s.OnStart(cmd.Process.Signal)
	}
	err := cmd.Wait()
	res.Stdout, res.Stderr = out.Bytes(), errb.Bytes()
	res.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	var exit *exec.ExitError
	switch {
	case err == nil, s.LeaveGroup && errors.Is(err, exec.ErrWaitDelay): // a job still holding the pipes open: the child itself exited 0
		res.ExitCode = 0
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		res.ExitCode = exit.ExitCode()
	}
	// The group may outlive its leader (a background grandchild): end it too, unless it may stay.
	if !s.LeaveGroup || ctx.Err() != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return res, nil
}
