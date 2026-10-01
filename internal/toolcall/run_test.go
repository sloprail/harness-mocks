package toolcall

import (
	"context"
	"testing"

	"github.com/sloprail/harness-mocks/internal/hooks"
)

func TestRunStopsAtTheFirstRefusal(t *testing.T) {
	var fired []string
	gate := func(name string, refuse bool) Gate {
		return func(context.Context) (bool, string) {
			fired = append(fired, name)
			return refuse, name + " says no"
		}
	}
	ran := false
	out, why := Run(context.Background(), []Gate{gate("a", false), gate("b", true), gate("c", false)},
		func(context.Context) hooks.ToolOutcome { ran = true; return hooks.ToolSucceeded })
	if out != hooks.ToolRefused || why != "b says no" || ran {
		t.Fatalf("Run = %v %q (ran %v), want refused by b without running", out, why, ran)
	}
	if len(fired) != 2 || fired[0] != "a" || fired[1] != "b" {
		t.Fatalf("gates fired %v, want [a b]: a later gate must not fire after a refusal", fired)
	}
}

func TestRunExecutesWhenNoGateRefuses(t *testing.T) {
	for _, want := range []hooks.ToolOutcome{hooks.ToolSucceeded, hooks.ToolFailed, hooks.ToolErrored} {
		out, why := Run(context.Background(), []Gate{func(context.Context) (bool, string) { return false, "" }},
			func(context.Context) hooks.ToolOutcome { return want })
		if out != want || why != "" {
			t.Errorf("Run = %v %q, want %v", out, why, want)
		}
	}
	if out, _ := Run(context.Background(), nil, func(context.Context) hooks.ToolOutcome { return hooks.ToolSucceeded }); out != hooks.ToolSucceeded {
		t.Errorf("a call with no gates must run, got %v", out)
	}
}
