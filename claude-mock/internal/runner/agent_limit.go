package runner

import (
	"context"
	"strconv"
)

// turnLimit is a sub-agent's `maxTurns`: the turns it may take, counting those
// in which it called a tool, and whether it stopped at the limit. A run that
// reaches it ends there, with no report and no SubagentStop (recorded:
// snapshots/runs/fgsub-maxturns: a sub-agent with maxTurns 2 ran two commands,
// and its Agent call returned at once with the note).
type turnLimit struct {
	max, turns int
	hit        bool
}

type turnLimitKey struct{}

// withTurnLimit is ctx carrying the limit of the sub-agent run under it; nil
// lifts the limit of an enclosing one for a run of its own.
func withTurnLimit(ctx context.Context, l *turnLimit) context.Context {
	return context.WithValue(ctx, turnLimitKey{}, l)
}

// Step counts a turn that called a tool and reports whether the run is now at
// its limit. A run with no limit is never at one.
func (l *turnLimit) Step() bool {
	if l == nil || l.max <= 0 {
		return false
	}
	l.turns++
	l.hit = l.turns >= l.max
	return l.hit
}

// reached reports whether the sub-agent stopped at its turn limit.
func (l *turnLimit) reached() bool { return l != nil && l.hit }

// turnLimitOf is the limit of the run ctx belongs to, nil when none.
func turnLimitOf(ctx context.Context) *turnLimit {
	l, _ := ctx.Value(turnLimitKey{}).(*turnLimit)
	return l
}

// definitionTurnLimit is the limit its definition sets (`maxTurns` in the
// frontmatter of the sub-agent's file), nil when it sets none.
//
// sr:provides foreground-subagent-result/claude
func definitionTurnLimit(cfg Config, name string) *turnLimit {
	n, err := strconv.Atoi(definitionField(cfg, name, "maxTurns"))
	if err != nil || n <= 0 {
		return nil
	}
	return &turnLimit{max: n}
}
