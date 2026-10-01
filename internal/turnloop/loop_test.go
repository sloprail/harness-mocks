package turnloop

import (
	"context"
	"strings"
	"testing"
)

// fake is an agent whose script's turns are given: each turn is a call key, or
// "" for the result that ends the run.
type fake struct {
	turns []string
	ran   []string
}

func (f *fake) Turn(context.Context) (Step[string], error) {
	if len(f.turns) == 0 || f.turns[0] == "" {
		return Step[string]{Done: true}, nil
	}
	k := f.turns[0]
	f.turns = f.turns[1:]
	return Step[string]{Call: k, Key: k}, nil
}

func (f *fake) Run(_ context.Context, call string) error { f.ran = append(f.ran, call); return nil }

// sr:proves turn-loop
func TestRunRunsEachCallThenStopsAtTheResult(t *testing.T) {
	f := &fake{turns: []string{"a", "b", ""}}
	if err := Run[string](context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.ran, ",") != "a,b" {
		t.Fatalf("calls run = %v, want [a b] in order, none after the result", f.ran)
	}
}

// sr:proves loop-guard
func TestFiveIdenticalTurnsInARowAbort(t *testing.T) {
	f := &fake{turns: []string{"x", "x", "x", "x", "x", "x"}}
	err := Run[string](context.Background(), f)
	if err == nil || !strings.Contains(err.Error(), "5 times in a row") {
		t.Fatalf("err = %v, want the loop guard's", err)
	}
	if len(f.ran) != MaxIdentical {
		t.Fatalf("the 5th identical call stops the run: %d ran, want %d", len(f.ran), MaxIdentical)
	}
}

// sr:proves loop-guard
func TestFourIdenticalThenADifferentCallNeverReachFive(t *testing.T) {
	f := &fake{turns: []string{"x", "x", "x", "x", "y", "x", "x", "x", "x", ""}}
	if err := Run[string](context.Background(), f); err != nil {
		t.Fatalf("4 in a row, a different call, then 4 again must not abort: %v", err)
	}
}
