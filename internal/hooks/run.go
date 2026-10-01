package hooks

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Command is one command handler of a hook.
type Command struct {
	// Line is the shell command line.
	Line string
	// Timeout is how long it may run; zero is the Runtime's default.
	Timeout time.Duration
	// Env is what only this command is told, KEY=VALUE, after the Runtime's.
	Env []string
}

// Outcome is how one hook command ended.
type Outcome struct {
	Command string
	// Exit is its exit status; -1 when it did not exit by itself.
	Exit int
	// Started is whether the command could be started at all.
	Started bool
	// TimedOut is whether its timeout stopped it.
	TimedOut       bool
	Stdout, Stderr string
	// Done is when it finished among the commands of its event: 1 for the
	// first to finish, and so on.
	Done int
	// Took is how long it ran.
	Took time.Duration
	// Timeout is the limit it ran under.
	Timeout time.Duration
}

// Runtime is where a harness runs its hook commands.
type Runtime struct {
	// Dir is the working directory of the session.
	Dir string
	// Env is the environment of a hook command (procexec.Env builds it).
	Env []string
	// DefaultTimeout applies to a command with none of its own.
	DefaultTimeout time.Duration
}

// RunAll runs the commands of the hooks that matched one event, all at once,
// each in its own process group with the event's payload on stdin, and returns
// their outcomes in the order given: no command waits for another, and none
// can keep another from starting.
func RunAll(ctx context.Context, cmds []Command, stdin []byte, rt Runtime) []Outcome {
	out := make([]Outcome, len(cmds))
	var finished atomic.Int64
	var wg sync.WaitGroup
	for i, c := range cmds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = runOne(ctx, c, stdin, rt)
			out[i].Done = int(finished.Add(1))
		}()
	}
	wg.Wait()
	return out
}

func runOne(ctx context.Context, c Command, stdin []byte, rt Runtime) Outcome {
	timeout := DefaultTimeout(c.Timeout, rt.DefaultTimeout)
	start := time.Now()
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{"/bin/sh", "-c", c.Line}, Dir: rt.Dir, Stdin: stdin, Env: append(append([]string{}, rt.Env...), c.Env...), Timeout: timeout,
	})
	return Outcome{Command: c.Line, Exit: res.ExitCode, Started: err == nil && res.Started, TimedOut: res.TimedOut,
		Stdout: string(res.Stdout), Stderr: string(res.Stderr), Took: time.Since(start), Timeout: timeout}
}
