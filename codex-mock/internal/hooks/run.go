package hooks

import (
	"context"
	"sync"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Invoker runs the hooks of one session.
type Invoker struct {
	Config Config
	// Dir is the session's working directory, where commands run.
	Dir string
	// Environ is the environment the mock was started with; Ident and
	// Defaults are the session facts Codex adds to a hook command's (none).
	Environ         []string
	Ident, Defaults map[string]string
	Common          Common
}

// Outcome is how one hook command ended.
type Outcome struct {
	Command        string
	Exit           int
	Started        bool
	TimedOut       bool
	Stdout, Stderr string
}

// defaultTimeout is Codex's hook timeout when none is configured.
//
// sr:docs https://developers.openai.com/codex/hooks#config-shape
const defaultTimeout = 600 * time.Second

// Fire runs every command of every group of the event whose matcher selects
// match, all at once (Codex launches matching hooks concurrently), and returns
// their outcomes in configuration order. The payload carries the common
// fields and the event's own.
//
// sr:docs https://developers.openai.com/codex/hooks#review-and-trust-hooks
func (iv *Invoker) Fire(ctx context.Context, ev Event, match string, own map[string]any) []Outcome {
	stdin := Payload(iv.Common, ev, own)
	var handlers []Handler
	for _, g := range iv.Config[ev] {
		if UnfilteredEvent(ev) || Matches(g.Matcher, match) {
			handlers = append(handlers, g.Handlers...)
		}
	}
	out := make([]Outcome, len(handlers))
	var wg sync.WaitGroup
	for i, h := range handlers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = iv.run(ctx, h, stdin)
		}()
	}
	wg.Wait()
	return out
}

func (iv *Invoker) run(ctx context.Context, h Handler, stdin []byte) Outcome {
	timeout := defaultTimeout
	if h.Timeout > 0 {
		timeout = time.Duration(h.Timeout) * time.Second
	}
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv:    []string{"/bin/sh", "-c", h.Command},
		Dir:     iv.Dir,
		Stdin:   stdin,
		Env:     procexec.Env(iv.Environ, iv.Ident, iv.Defaults),
		Timeout: timeout,
	})
	o := Outcome{Command: h.Command, Exit: res.ExitCode, Started: err == nil && res.Started, TimedOut: res.TimedOut}
	o.Stdout, o.Stderr = string(res.Stdout), string(res.Stderr)
	return o
}
