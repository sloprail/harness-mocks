package tasks

import (
	"context"
	"io"
	"os"
	"time"

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

// StartYielding starts a command as StartCommand does and waits up to wait for
// it: ended reports whether it ended within that time. A command that has not
// is left running, for the caller to answer with a receipt (background-bash:
// what a harness does of a command it lets go on after a yield time).
func (r *Registry) StartYielding(ctx context.Context, t *Task, s CommandSpec, wait time.Duration) (ended bool, err error) {
	if err := r.StartCommand(t, s); err != nil {
		return false, err
	}
	select {
	case <-t.Done():
		return true, nil
	case <-time.After(wait):
	case <-ctx.Done():
	}
	return false, nil
}

// AwaitEnds waits, until ctx ends, for each of launched to end, and returns
// the ones that did, in launch order: what a non-interactive run does before
// it finishes when it waits for its background commands.
func AwaitEnds(ctx context.Context, launched []*Task) []*Task {
	var ended []*Task
	for _, t := range launched {
		select {
		case <-t.Done():
			ended = append(ended, t)
		case <-ctx.Done():
		}
	}
	return ended
}

// AfterHookFires reports whether the after-tool hooks fire for a call whose
// command was let go: not while it still runs, the end of the command being
// what they would report.
func AfterHookFires(stillRunning bool) bool { return !stillRunning }
