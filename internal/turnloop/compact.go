package turnloop

import (
	"context"
	"errors"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Compactor is a Host that compacts the session when the script asks for it;
// a host without it ignores the request. An error ends the turn, and the run
// with it: ErrAborted is the harness aborting the turn.
type Compactor interface {
	Compact(ctx context.Context, trigger string) error
}

// FieldCompactor is a Compactor that is told the compaction whole, the other keys
// of the script's compact line too (a harness whose hook says how big the context was).
type FieldCompactor interface {
	CompactWith(ctx context.Context, c scenario.Compact) error
}

// compact carries out the compaction the turn ended with.
func compact(ctx context.Context, h Host, c scenario.Compact) error {
	if f, ok := h.(FieldCompactor); ok {
		return f.CompactWith(ctx, c)
	}
	if k, ok := h.(Compactor); ok {
		return k.Compact(ctx, c.Trigger)
	}
	return nil
}

// ErrAborted is a host ending the turn without going on to its end-of-turn
// hooks: the turn was aborted.
var ErrAborted = errors.New("the turn was aborted")

// ErrTurnEnded is a host ending the turn early but cleanly: the turn completes
// as it would at its end, only the model is not asked again and no end-of-turn
// hook fires (a session-start hook that followed a compaction said to stop).
var ErrTurnEnded = errors.New("the turn ended early")
