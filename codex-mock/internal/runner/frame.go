package runner

import (
	"fmt"
	"time"
)

// failedFrame is what the harness tells the agent before the reason a hook gave for rejecting a tool
// call of its script, which makes the call throw: "Script failed", the time, "Output:" (recorded:
// runs/posttool-block, runs/pretool-decisions, runs/stops).
func failedFrame(wall time.Duration) string {
	return fmt.Sprintf("Script failed\nWall time %.1f seconds\nOutput:\n", wall.Seconds())
}

// completedFrame is what the harness tells the agent before the output of a command that ran to
// its end, whatever its exit status: the time it took, to a tenth of a second (recorded: every
// shell result of runs/shell-exit-status, "Script completed\nWall time 0.1 seconds\nOutput:\n").
func completedFrame(wall time.Duration) string {
	return fmt.Sprintf("Script completed\nWall time %.1f seconds\nOutput:\n", wall.Seconds())
}
