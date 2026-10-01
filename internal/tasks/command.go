package tasks

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// CommandSpec is a shell command to run in the background.
type CommandSpec struct {
	// Argv is the program and its arguments; Dir and Env its working directory
	// and whole environment.
	Argv []string
	Dir  string
	Env  []string
	// Out is the file the command's stdout and stderr go to; it is closed when
	// the command ends.
	Out *os.File
	// Trailer is what is appended to Out when the command ends: how it ended
	// (the exit code, or that it was killed). Nil appends nothing.
	Trailer func(exitCode int, killed bool) string
	// Started runs once the task is registered, before anything of its end can
	// happen; Ended runs once the command has ended and Out is closed, before
	// the task is finished.
	Started, Ended func(*Task)
}

// StartCommand starts the command for t in the background and registers t. The
// command is not bound to the turn that started it, so it keeps running while
// the agent works; it has its own process group, so ending it reaches whatever
// it spawned. t.ExitCode is set when it ends.
func (r *Registry) StartCommand(t *Task, s CommandSpec) error {
	cmd := exec.Command(s.Argv[0], s.Argv[1:]...) //nolint:gosec
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	cmd.Stdout, cmd.Stderr = s.Out, s.Out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		s.Out.Close()
		return err
	}
	t.Kind = Command
	t.kill = func() { killGroup(cmd) }
	r.Add(t)
	if s.Started != nil {
		s.Started(t)
	}
	r.Go(func() {
		code := 0
		if err := cmd.Wait(); err != nil {
			code = 1
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
				code = cmd.ProcessState.ExitCode()
			}
		}
		t.ExitCode = code
		if s.Trailer != nil {
			io.WriteString(s.Out, s.Trailer(code, t.Killed())) //nolint:errcheck
		}
		s.Out.Close()
		if s.Ended != nil {
			s.Ended(t)
		}
		r.Finish(t)
	})
	return nil
}

// StartAgent registers a background agent t and runs it concurrently with the
// turn that launched it: run returns when the agent has ended, and the caller
// has already answered the launch.
func (r *Registry) StartAgent(t *Task, run func(ctx context.Context)) {
	t.Kind = Agent
	r.Add(t)
	r.Go(func() {
		run(r.ctx)
		r.Finish(t)
	})
}

// killGroup kills a command's whole process group.
func killGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = cmd.Process.Kill()
	}
}
