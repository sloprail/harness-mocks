package runner

import "errors"

// errRunFailed ends a run whose result frame says it failed: the process exits
// non-zero, as the real claude does when the model API fails (recorded:
// snapshots/runs/run-failure, an unrecognised model: exit 1, an error result on
// stdout). Whether a Stop follows is the core's (scenario.Result.Failed).
var errRunFailed = errors.New("claude-mock: the run failed: its result frame is an error")
