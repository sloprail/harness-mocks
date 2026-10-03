package tasks

import (
	"context"
	"io"
	"os"

	"github.com/sloprail/harness-mocks/internal/procexec"
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
	p, err := procexec.Start(procexec.Spec{Argv: s.Argv, Dir: s.Dir, Env: s.Env}, s.Out)
	if err != nil {
		s.Out.Close()
		return err
	}
	t.Kind = Command
	t.kill, t.Pid = p.Kill, p.Pid()
	r.Add(t)
	if s.Started != nil {
		s.Started(t)
	}
	r.Go(func() {
		t.ExitCode = p.Wait()
		if s.Trailer != nil {
			io.WriteString(s.Out, s.Trailer(t.ExitCode, t.Killed())) //nolint:errcheck
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
//
// sr:capability background-agent
func (r *Registry) StartAgent(t *Task, run func(ctx context.Context)) {
	t.Kind = Agent
	r.Add(t)
	r.Go(func() {
		run(r.ctx)
		r.Finish(t)
	})
}

// RunsInBackground reports whether a command or sub-agent that asked to run in
// the background does: it does not when the harness has background tasks
// turned off, and then it runs in the foreground like any other. It is the
// entry to background-bash: StartCommand runs what this lets through.
//
// sr:capability background-bash
func RunsInBackground(asked, disabled bool) bool {
	return asked && !disabled
}
