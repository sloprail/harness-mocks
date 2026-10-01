package hooks

import (
	"context"
	"sync"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Command is one command handler of a hook.
type Command struct {
	// Line is the shell command line.
	Line string
	// Timeout is how long it may run; zero is the Runtime's default.
	Timeout time.Duration
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
	var wg sync.WaitGroup
	for i, c := range cmds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = runOne(ctx, c, stdin, rt)
		}()
	}
	wg.Wait()
	return out
}

func runOne(ctx context.Context, c Command, stdin []byte, rt Runtime) Outcome {
	timeout := rt.DefaultTimeout
	if c.Timeout > 0 {
		timeout = c.Timeout
	}
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{"/bin/sh", "-c", c.Line}, Dir: rt.Dir, Stdin: stdin, Env: rt.Env, Timeout: timeout,
	})
	return Outcome{Command: c.Line, Exit: res.ExitCode, Started: err == nil && res.Started, TimedOut: res.TimedOut,
		Stdout: string(res.Stdout), Stderr: string(res.Stderr)}
}
