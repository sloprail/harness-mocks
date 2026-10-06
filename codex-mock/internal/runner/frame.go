package runner

import (
	"fmt"
	"time"
)

// completedFrame is what the harness tells the agent before the output of a command that ran to
// its end, whatever its exit status: the time it took, to a tenth of a second (recorded: every
// shell result of runs/shell-exit-status, "Script completed\nWall time 0.1 seconds\nOutput:\n").
func completedFrame(wall time.Duration) string {
	return fmt.Sprintf("Script completed\nWall time %.1f seconds\nOutput:\n", wall.Seconds())
}
