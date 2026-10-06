package turnloop

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Interleaver is a Host that has the several calls of one turn in flight
// together: each is started before any is completed. A host without it carries
// out the first call of a turn only, as it always did.
type Interleaver interface {
	// Start begins a call (what the harness shows as the call starting) and
	// returns what completes it.
	Start(ctx context.Context, tu scenario.ToolUse) (finish func())
}

// Orderer is an Interleaver that starts and completes the calls of a response in an
// order of its own, not the one the model made them in.
type Orderer interface {
	// Order is the calls of one response in the order the harness takes them.
	Order(calls []scenario.ToolUse) []scenario.ToolUse
}

// turnCalls is the calls of the turn the host carries out: all of them for an
// Interleaver, the first otherwise.
func turnCalls(h Host, t scenario.Turn) []scenario.ToolUse {
	if _, ok := h.(Interleaver); ok || len(t.Tools) == 0 {
		return t.Tools
	}
	return t.Tools[:1]
}

// callsKey names a turn's calls, to tell a script that asks for the same ones
// again.
func callsKey(calls []scenario.ToolUse) string {
	key := ""
	for _, c := range calls {
		key += c.Name + string(c.Input)
	}
	return key
}

// perform carries out the turn's calls: one at a time through Tool, and when
// there are several (only an Interleaver is given them), all started in order
// before any is completed, in the same order.
func perform(ctx context.Context, h Host, calls []scenario.ToolUse) {
	if o, ok := h.(Orderer); ok && len(calls) > 1 {
		calls = o.Order(calls)
	}
	if len(calls) == 1 {
		h.Tool(ctx, calls[0])
		return
	}
	var finish []func()
	for _, c := range calls {
		finish = append(finish, h.(Interleaver).Start(ctx, c))
	}
	for _, f := range finish {
		f()
	}
}
