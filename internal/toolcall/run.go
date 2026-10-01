// Package toolcall is the harness-neutral core of one tool call of the agent.
package toolcall

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/hooks"
)

// Gate is one before-tool stage of a call: the hooks a harness fires before
// the tool runs (some harnesses have more than one such stage). It reports
// whether it refuses the call, and why.
type Gate func(ctx context.Context) (refused bool, reason string)

// Run carries out one call: the gates in order, the first refusal ending it
// (the later gates do not fire and the tool does not run; the reason is the
// refusal's), otherwise the tool, whose outcome is returned.
func Run(ctx context.Context, gates []Gate, execute func(ctx context.Context) hooks.ToolOutcome) (outcome hooks.ToolOutcome, refusal string) {
	for _, g := range gates {
		if refused, reason := g(ctx); refused {
			return hooks.ToolRefused, reason
		}
	}
	return execute(ctx), ""
}
