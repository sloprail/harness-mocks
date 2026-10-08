package runner

import (
	"context"

	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// SubmitPrompt fires nothing in print mode (recorded: cursor-agent -p never fires
// beforeSubmitPrompt); a TUI session fires it (interactive.go).
func (s *session) SubmitPrompt(ctx context.Context) (string, bool) {
	if s.cfg.Interactive {
		return s.submitPrompt(ctx)
	}
	return "", false
}

// Say prints what the agent said, and records it; a TUI session tells it to
// afterAgentResponse when the turn ends.
func (s *session) Say(text string) {
	s.texts = append(s.texts, text)
	s.tr.text(text)
	s.pending = append(s.pending, text)
	if s.firesStop() {
		s.said = append(s.said, text)
	}
}

// EndOfTurn fires no hook in print mode (recorded: cursor-agent -p never fires stop), unless
// run with the opt-in; what can continue the turn is a background shell's end (afterTurn).
//
// What the agent said in the turn is not brought out when the turn ends: a
// turn a finished background shell gives the agent follows, and everything said
// since the last call comes out as one frame at the end of the run, after the
// task's notification (recorded: runs/background-bash-start,
// runs/bg-bash-reaped-at-exit).
func (s *session) EndOfTurn(ctx context.Context, _ string, _ bool) (string, bool) {
	if s.cfg.Interactive {
		return s.stopped(ctx)
	}
	if s.cfg.Stop && s.owner == "" { // the opt-in: the TUI's stop, in print mode (interactive.go)
		if follow, again := s.stopped(ctx); again {
			return follow, true
		}
	}
	if s.owner != "" { // a sub-agent ends with its final response: its parent goes on
		return "", false
	}
	return s.afterTurn(ctx)
}

// Continue records the turn a finished background shell gives the agent, as the
// user message it is in the transcript.
func (s *session) Continue(prompt string) {
	s.tr.user(prompt)
	s.requestID, s.modelN = coresession.NewID(), 0 // a turn of its own is a model request of its own
	if s.stopFollowUp {                            // the follow-up a stop hook gave
		s.followedUp()
	}
}

// CapOverridden: a stop hook asked for more than the cap; the turn ends.
func (s *session) CapOverridden(int) {}

// SessionFile is the conversation's transcript so far.
func (s *session) SessionFile() string { return s.tr.path }
