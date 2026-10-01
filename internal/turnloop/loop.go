// Package turnloop is the harness-neutral core of a scripted run's turns.
package turnloop

import (
	"context"
	"fmt"
)

// MaxIdentical is how many turns in a row a script may end on the same call
// before the run is aborted.
const MaxIdentical = 5

// Step is how one turn of the script ended.
type Step[C any] struct {
	// Done: the script ended the run (its result line), or printed no call.
	Done bool
	// Call is the tool call the turn ended on, when it did not end the run.
	Call C
	// Key identifies the call: two turns that end on the same call have the
	// same key.
	Key string
}

// Agent is a harness's side of a scripted run.
type Agent[C any] interface {
	// Turn runs the script once, handles what it printed and says how the
	// turn ended.
	Turn(ctx context.Context) (Step[C], error)
	// Run carries out the call a turn ended on.
	Run(ctx context.Context, call C) error
}

// Run drives the agent turn by turn: the script runs once per turn and prints
// stream-json lines; a tool call line ends the turn, the mock carries out the
// call and runs the script again; a result line ends the run.
//
// A script that ends 5 turns in a row on the same call makes the run abort
// with an error, so a stuck scenario cannot loop forever.
//
// sr:invariant turn-loop
// sr:invariant loop-guard
func Run[C any](ctx context.Context, a Agent[C]) error {
	last, same := "", 0
	for {
		step, err := a.Turn(ctx)
		if err != nil {
			return err
		}
		if step.Done {
			return nil
		}
		if err := a.Run(ctx, step.Call); err != nil {
			return err
		}
		if step.Key != "" && step.Key == last {
			same++
		} else {
			last, same = step.Key, 1
		}
		if same >= MaxIdentical {
			return fmt.Errorf("scenario looped: the same tool call was emitted %d times in a row without advancing (%s); the script runs once per turn and must vary its output, reading A10N_MOCK_SESSION_FILE", MaxIdentical, step.Key)
		}
	}
}
