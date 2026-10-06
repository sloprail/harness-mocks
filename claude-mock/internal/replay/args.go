package replay

import (
	"encoding/json"
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// modelled are the flags of a recording's setup/args that the mock models, with
// how many values each takes; the replay passes them to the mock as they were
// given to claude.
var modelled = map[string]int{"--max-turns": 1, "--model": 1, "--include-hook-events": 0}

// refused are the flags the mock refuses (adr/fail-fast-unimplemented): a
// recording made with one cannot be replayed, and says so rather than being
// listed as a replay that fails (see RefusedPrefix).
var refused = map[string]bool{
	"--max-budget-usd": true, "--input-format": true,
	"--include-partial-messages": true, "--agent": true, "--bare": true,
	"--no-such-flag": true, // a flag claude itself does not know (recorded: snapshots/runs/invalid-flag)
}

// RefusedPrefix starts the reason of a recording the mock refuses by design.
const RefusedPrefix = "the mock refuses "

// parseArgs are the flags of a setup/args file (one argument per line) as
// arguments for the mock. A flag the mock refuses is an Unbuildable that says
// so, one it does not model at all an Unbuildable that says that.
func parseArgs(text string) ([]string, error) {
	var out []string
	f := strings.Fields(text)
	for i := 0; i < len(f); {
		flag := f[i]
		if refused[flag] {
			return nil, unbuildable(fmt.Errorf("%s%s", RefusedPrefix, flag))
		}
		n, ok := modelled[flag]
		if !ok {
			return nil, unbuildable(fmt.Errorf("the setup's args have %s, which the adapter does not map to a mock flag", flag))
		}
		if i+n >= len(f) {
			return nil, unbuildable(fmt.Errorf("the setup's args end after %s, short of its value", flag))
		}
		out = append(out, f[i:i+1+n]...)
		i += 1 + n
	}
	return out, nil
}

// wantExit is the exit status the recorded run ended with, which the mock's
// run must end with too.
func wantExit(rec core.Recording) int {
	if rec.Setup["exit"] == "1" {
		return 1
	}
	return 0
}

// failedResult is the fields of the recorded run's last result frame that say its
// model API failed, as JSON for the scenario's own result frame ("" when it did
// not): the mock has no API to fail, so the scenario says it (recorded:
// snapshots/runs/run-failure).
func failedResult(stream []map[string]any) string {
	var last map[string]any
	for _, f := range stream {
		if f["type"] == "result" {
			last = f
		}
	}
	if last == nil || last["is_error"] != true {
		return ""
	}
	extra := map[string]any{"is_error": true, "api_error_status": last["api_error_status"], "terminal_reason": last["terminal_reason"], "stop_reason": last["stop_reason"]}
	// and the assistant frame the harness wrote for it: flagged as an API error message
	for _, f := range stream {
		if f["is_api_error_message"] == true {
			msg, _ := f["message"].(map[string]any)
			extra["assistant"] = map[string]any{"error": f["error"], "is_api_error_message": true, "stop_reason": msg["stop_reason"], "stop_sequence": msg["stop_sequence"]}
		}
	}
	b, _ := json.Marshal(extra)
	return string(b)
}
