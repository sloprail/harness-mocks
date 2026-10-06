package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// interrupted is what follows a turn the user interrupted (SIGINT, as a Ctrl-C does a headless run)
// while the agent was at work: the Interrupt hooks fire, the agent is told the user interrupted the
// turn, the turn is recorded as aborted and the stream prints nothing more, and no Stop hook runs;
// the session still ends (recorded: runs/interrupt-hook). The call the user interrupted was
// answered "aborted by user after <time>" (Execute).
//
// sr:provides hook-matcher-filter/codex
func (h *state) interrupted(ctx context.Context) error {
	h.hooks.Fire(ctx, hooks.Interrupt, "", map[string]any{"turn_id": h.turnID})
	h.rollout.UserInterrupted()
	h.rollout.TurnAborted(h.turnID)
	h.events.Abort()
	return turnloop.ErrAborted
}
