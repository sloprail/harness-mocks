package turnloop

import (
	"context"
	"errors"
)

// Compactor is a Host that compacts the session when the script asks for it;
// a host without it ignores the request. An error ends the turn, and the run
// with it: ErrAborted is the harness aborting the turn.
type Compactor interface {
	Compact(ctx context.Context, trigger string) error
}

// ErrAborted is a host ending the turn without going on to its end-of-turn
// hooks: the turn was aborted.
var ErrAborted = errors.New("the turn was aborted")
