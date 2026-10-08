package toolexec

import "time"

// Unapproved is a Shell call in a run that was not given --force or --yolo:
// no one is there to approve it, so it is rejected and does not run. Its
// completed frame is a rejection with no reason, from the workspace; the hooks
// that follow a command still fire, with what a command that printed nothing
// and exited 0 would give them (recorded: runs/noninteractive-no-force).
//
// sr:docs https://cursor.com/docs/cli/headless#how-it-works
func Unapproved(c Call, dir string) Result {
	start := time.Now()
	return Result{
		Took: max(time.Since(start), time.Microsecond), // the time it took to refuse, as every call reports its own
		Frame: map[string]any{"rejected": map[string]any{
			"command": c.Command(), "workingDirectory": dir, "reason": "", "isReadonly": false,
		}},
		ToolOutput: jsonString(struct {
			Output   string `json:"output"`
			ExitCode int    `json:"exitCode"`
		}{"", 0}),
	}
}
