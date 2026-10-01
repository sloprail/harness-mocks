package tasks

import (
	"context"
	"time"
)

// DefaultWaitCeiling is how long a non-interactive run waits idle for background
// agents after its final turn before it stops waiting.
const DefaultWaitCeiling = 10 * time.Minute

// WaitCeiling is ctx limited to ceiling of idle waiting: past it the wait ends and
// what still runs is dropped (Shutdown). A ceiling of zero or less is no limit.
// Idle waiting starts over each time the agent takes a turn to handle a result, so
// each wait gets its own.
func WaitCeiling(ctx context.Context, ceiling time.Duration) (context.Context, context.CancelFunc) {
	if ceiling <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, ceiling)
}
