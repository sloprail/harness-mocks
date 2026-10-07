package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	"github.com/sloprail/harness-mocks/internal/compaction"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// Compact compacts the session, the way Codex does (recorded under
// codex-mock/snapshots/runs/manual-compaction-auto, -auto-blocked and
// -auto-post-stopped): PreCompact fires with the trigger; a hook printing
// `continue: false` stops the compaction, and nothing more happens. Otherwise
// the rollout records the compaction and PostCompact fires; unless that hook
// printed `continue: false`, SessionStart with source "compact" follows. A hook
// that stopped it, before or after, also aborts the turn: the rollout records
// it interrupted, the stream prints nothing more, no Stop hook fires and the
// run fails (turnloop.ErrAborted). The scenario's compact record names the
// trigger, "manual" when it names none.
//
// sr:provides manual-compaction/codex
// sr:docs https://developers.openai.com/codex/hooks#precompact
// sr:docs https://developers.openai.com/codex/hooks#postcompact
func (h turnHost) Compact(ctx context.Context, trigger string) error {
	if trigger == "" {
		trigger = "manual"
	}
	s := h.state
	own := map[string]any{"turn_id": s.turnID, "trigger": trigger}
	stops := func(ev hooks.Event) (stop bool) {
		for _, o := range s.hooks.Fire(ctx, ev, trigger, own) {
			stop = stop || hooks.Stops(o)
		}
		return stop
	}
	startStopped := false
	res := compaction.Do(false, compaction.Steps{
		Before: func() bool { return stops(hooks.PreCompact) },
		Boundary: func() {
			s.rollout.Compacted(session.Turn{SessionID: s.id, TurnID: s.turnID, Cwd: s.cfg.Cwd, Model: s.cfg.Model, Sandbox: s.cfg.Sandbox})
		},
		Summary:    func() {},
		AfterStops: func() bool { return stops(hooks.PostCompact) },
		Resume: func() {
			for _, o := range s.hooks.Fire(ctx, hooks.SessionStart, "compact", map[string]any{"source": "compact"}) {
				startStopped = startStopped || hooks.Stops(o)
				if d := hooks.Interpret(hooks.SessionStart, o); d.Context != "" {
					s.rollout.Developer(d.Context)
				}
			}
		},
		ResumeLast: true,
	})
	if res.Stopped {
		s.rollout.TurnAborted(s.turnID)
		s.events.Abort()
		return turnloop.ErrAborted
	}
	// A session-start hook of the compaction that printed `continue: false` ends
	// the turn, not the session, as it does at startup (runs/session-start-compact-continue-false).
	if corehooks.StartHookEndsTurn(startStopped, true) {
		return turnloop.ErrTurnEnded
	}
	return nil
}
