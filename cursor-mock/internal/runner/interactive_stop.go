package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// stopped fires the stop hook when the agent's turn ends, after the response hook: a hook that
// answers with a followup_message has it submitted as the next message (the last such answer
// wins, as the docs say of merged responses).
//
// sr:provides stop-block-continuation/cursor
// sr:provides stop-hook-payload/cursor
// sr:docs https://cursor.com/docs/hooks#stop
func (s *session) stopped(ctx context.Context) (string, bool) {
	s.responded()
	follow := ""
	for _, d := range s.hooks.Fire(ctx, hooks.Stop, "Stop", s.withUsage(map[string]any{"status": "completed", "loop_count": s.loops})) {
		if d.Followup != "" {
			follow = d.Followup
		}
	}
	s.stopFollowUp = follow != ""
	return follow, follow != ""
}

// firesStop: the session fires the TUI's end-of-turn hooks, being a TUI session or a print-mode
// one run with the opt-in (Config.Stop).
func (s *session) firesStop() bool { return s.cfg.Interactive || s.cfg.Stop }

// followedUp records the follow-up a stop hook gave as the message that goes on with the turn: a
// generation of its own, with no beforeSubmitPrompt (recorded: runs/tui-stop-followup).
func (s *session) followedUp() {
	s.stopFollowUp = false
	s.loops++
	s.gen = coresession.NewID()
}

// blockCap is how many follow-ups in a row end the turn: only a stop hook blocks, so none without one.
func (s *session) blockCap() int {
	if s.cfg.Stop {
		return stopLoopLimit
	}
	return 0
}
