package runner

import (
	"encoding/json"
	"errors"
)

// errRunFailed ends a run whose result frame says it failed: the process exits
// non-zero, as the real claude does when the model API fails (recorded:
// snapshots/runs/run-failure, an unrecognised model: exit 1, an error result on
// stdout).
var errRunFailed = errors.New("claude-mock: the run failed: its result frame is an error")

// resultFailed is whether a result frame reports a failed run (is_error true).
// Such a run fires no Stop (the real one fires StopFailure, which the mock does
// not model) and still ends with SessionEnd.
func resultFailed(line []byte) bool {
	var frame struct {
		IsError bool `json:"is_error"`
	}
	return len(line) > 0 && json.Unmarshal(line, &frame) == nil && frame.IsError
}
