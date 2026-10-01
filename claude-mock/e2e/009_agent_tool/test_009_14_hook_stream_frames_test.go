package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamFrames is every JSON line the run streamed, in order.
func streamFrames(out string) []map[string]any {
	var frames []map[string]any
	for _, l := range strings.Split(out, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil {
			frames = append(frames, m)
		}
	}
	return frames
}

// systemFrames is the streamed system frames of one subtype.
func systemFrames(out, subtype string) []map[string]any {
	var got []map[string]any
	for _, f := range streamFrames(out) {
		if f["type"] == "system" && f["subtype"] == subtype {
			got = append(got, f)
		}
	}
	return got
}

// runSessionStart runs a one-turn scenario under a SessionStart hook and
// returns what the run streamed.
func runSessionStart(t *testing.T, stderr string, code int) string {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]string{"SessionStart": hookWithRaw(t, dir, "", stderr, code)})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, rc := runInDir(t, dir, nil, "--script", script, "--session-id", "s-ssf", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, rc, "output:\n%s", out)
	return out
}

// A SessionStart hook's run is streamed first: hook_started, then
// hook_response with the same hook_id, its stdout, stderr, output, exit code
// and outcome "error" for a failing status (recorded:
// snapshots/runs/hook-exit-codes, exit 2).
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_14_SessionStartFailureIsStreamed(t *testing.T) {
	out := runSessionStart(t, "session-start stderr on exit 2", 2)
	frames := streamFrames(out)
	require.GreaterOrEqual(t, len(frames), 2, "output:\n%s", out)
	started, resp := frames[0], frames[1]
	assert.Equal(t, "hook_started", started["subtype"])
	assert.Equal(t, "hook_response", resp["subtype"])
	for _, f := range []map[string]any{started, resp} {
		assert.Equal(t, "SessionStart:startup", f["hook_name"])
		assert.Equal(t, "SessionStart", f["hook_event"])
	}
	assert.Equal(t, started["hook_id"], resp["hook_id"])
	assert.Equal(t, float64(2), resp["exit_code"])
	assert.Equal(t, "error", resp["outcome"])
	assert.Equal(t, "session-start stderr on exit 2\n", resp["stderr"])
	assert.Equal(t, "session-start stderr on exit 2\n", resp["output"])
	assert.Equal(t, "", resp["stdout"])
}

// A SessionStart hook that succeeds streams the same pair with outcome
// "success" and exit code 0 (recorded: snapshots/runs/subprocess-session-env).
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-output
// sr:proves hook-exit-code-semantics/claude
func TestT009_14_SessionStartSuccessIsStreamed(t *testing.T) {
	out := runSessionStart(t, "", 0)
	resp := systemFrames(out, "hook_response")
	require.Len(t, resp, 1, "output:\n%s", out)
	assert.Equal(t, float64(0), resp[0]["exit_code"])
	assert.Equal(t, "success", resp[0]["outcome"])
	assert.Len(t, systemFrames(out, "hook_started"), 1)
}

// runStopStream runs a one-turn scenario under a Stop hook and returns what
// the run streamed.
func runStopStream(t *testing.T, stopHook func(dir string) string) string {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]string{"Stop": stopHook(dir)})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, rc := runInDir(t, dir, nil, "--script", script, "--session-id", "s-stf", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, rc, "output:\n%s", out)
	return out
}

// assertStopHookError checks the run streamed exactly n stop-hook-error
// notifications, as recorded.
func assertStopHookError(t *testing.T, out string, n int) {
	t.Helper()
	var got []map[string]any
	for _, f := range systemFrames(out, "notification") {
		if f["key"] == "stop-hook-error" {
			got = append(got, f)
		}
	}
	require.Len(t, got, n, "output:\n%s", out)
	for _, f := range got {
		assert.Equal(t, "Stop hook error occurred · ctrl+o to see", f["text"])
		assert.Equal(t, "immediate", f["priority"])
	}
}

// A Stop hook that blocks with exit 2 streams one stop-hook-error
// notification; the re-run's Stop, exiting 0, streams none (recorded:
// snapshots/runs/hook-exit-codes).
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_14_StopBlockStreamsHookError(t *testing.T) {
	out := runStopStream(t, func(dir string) string { return stopOnce(t, dir, "", "reply STOPPED-ONCE", 2) })
	assertStopHookError(t, out, 1)
}

// A Stop hook that fails with exit 1 streams the same notification (recorded:
// snapshots/runs/hook-exit-json).
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_14_StopFailureStreamsHookError(t *testing.T) {
	out := runStopStream(t, func(dir string) string { return hookWithRaw(t, dir, "", "stop hook failed with exit 1", 1) })
	assertStopHookError(t, out, 1)
}

// A Stop hook that succeeds streams no notification.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-output
// sr:proves hook-exit-code-semantics/claude
func TestT009_14_StopSuccessStreamsNoHookError(t *testing.T) {
	out := runStopStream(t, func(dir string) string { return hookWithRaw(t, dir, "", "", 0) })
	assertStopHookError(t, out, 0)
	assert.Empty(t, systemFrames(out, "hook_started"), "only SessionStart's runs are streamed")
}
