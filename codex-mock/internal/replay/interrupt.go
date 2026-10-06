package replay

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// signalWhenStarted is a stdout that has the run sent SIGINT when its stream shows a command started.
type signalWhenStarted struct {
	mu     sync.Mutex
	signal func(os.Signal) error
	sent   bool
}

func (w *signalWhenStarted) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.sent && w.signal != nil && strings.Contains(string(p), `"command_execution"`) && strings.Contains(string(p), `"in_progress"`) {
		w.sent = true
		_ = w.signal(syscall.SIGINT)
	}
	return len(p), nil
}

// runInterrupted runs the mock as the user interrupted it: SIGINT is sent when its stream shows the
// command the agent ran has started, an event, not a time, and the run is expected to end with
// status 1, as the recorded one did (runs/interrupt-hook).
func runInterrupted(argv []string, dir string, env []string) (string, error) {
	w := &signalWhenStarted{}
	res, err := procexec.Run(context.Background(), procexec.Spec{
		Argv: argv, Dir: dir, Env: env, Stdout: w,
		OnStart: func(signal func(os.Signal) error) {
			w.mu.Lock()
			w.signal = signal
			w.mu.Unlock()
		},
	})
	if err != nil || !w.sent || res.ExitCode != 1 {
		return "", &core.MockFailure{Detail: fmt.Sprintf("an interrupted run (interrupted: %v) must end with status 1: %v (exit %d) %s", w.sent, err, res.ExitCode, res.Stderr)}
	}
	return string(res.Stdout), nil
}
