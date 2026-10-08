package subagents

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// HoldExec holds the carrying out of a step's call until what its gate names for that moment has
// happened: the agent that started it (parent) having taken as many steps as the gate says.
func HoldExec(ctx context.Context, g scenario.Gate, parent *Progress, ancestors []*Progress) {
	if g.ExecParentSteps > 0 && parent != nil {
		parent.WaitSteps(ctx, g.ExecParentSteps)
	}
	for i, n := range g.ExecAncestorSteps {
		if n > 0 && i < len(ancestors) && ancestors[i] != nil {
			ancestors[i].WaitSteps(ctx, n)
		}
	}
}

// HoldAncestors holds an agent's step back until the agents above the one that started it have finished
// as many calls as its gate says.
func HoldAncestors(ctx context.Context, g scenario.Gate, ancestors []*Progress) {
	for i, n := range g.AncestorDone {
		if n > 0 && i < len(ancestors) && ancestors[i] != nil {
			ancestors[i].Wait(ctx, 0, n)
		}
	}
}
