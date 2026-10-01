package hooks

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// runHandlers runs every handler of one event together, as Claude Code does
// (docs: "All matching hooks run in parallel"), and returns each handler's run
// in the order given, with what the command handlers did behind them. Only
// the command handlers carry an Outcome; an HTTP handler's slot is zero.
func (inv *Invoker) runHandlers(ctx context.Context, handlers []HandlerSpec, ev EventName, hookCwd string, payload []byte) ([]HandlerRun, []corehooks.Outcome) {
	runs := make([]HandlerRun, len(handlers))
	outs := make([]corehooks.Outcome, len(handlers))
	var cmds []corehooks.Command
	var at []int
	var wg sync.WaitGroup
	for i, h := range handlers {
		switch h.Type {
		case "command":
			if strings.TrimSpace(h.Command) == "" {
				continue
			}
			// sr:provides hook-command-handler/claude
			cmds = append(cmds, corehooks.Command{Line: strings.TrimSpace(h.Command), Timeout: time.Duration(h.Timeout) * time.Second})
			at = append(at, i)
		case "http":
			wg.Add(1)
			go func() {
				defer wg.Done()
				runs[i] = inv.httpRun(ctx, h, ev, payload)
			}()
		default:
			slog.Debug("hooks: unsupported handler type", "type", h.Type)
		}
	}
	// Mirror the real claude CLI's hook environment (see NewInvoker): the
	// session's identity, whether or not a session id is known.
	// sr:provides hook-timeout/claude
	rt := corehooks.Runtime{Dir: hookCwd, Env: hookEnv(inv.sessionID), DefaultTimeout: defaultTimeout(ev)}
	for k, o := range corehooks.RunAll(ctx, cmds, payload, rt) {
		outs[at[k]] = o
		runs[at[k]] = commandRun(handlers[at[k]], ev, o)
	}
	wg.Wait()
	return runs, outs
}
