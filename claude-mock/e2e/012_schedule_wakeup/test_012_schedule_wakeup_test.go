package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSessionStartHook installs a SessionStart hook that:
//   - appends the "source" field of every SessionStart fire to logFile, and
//   - on source="compact" outputs additionalContext (so the mock surfaces it as a
//     system record on the output stream, mirroring real claude's compact re-seed).
//
// ScheduleWakeup must NEVER cause a compact fire (it is a plain resume tool), so the
// ScheduleWakeup tests use this hook to PROVE compact does not appear.
func writeSessionStartHook(t *testing.T, dir, logFile string) {
	t.Helper()
	hook := filepath.Join(dir, "session_start.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
src=$(echo "$input" | grep -o '"source":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "$src" >> "`+logFile+`"
if [ "$src" = "compact" ]; then
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"REINJECTED-MEMORY"}}'
fi
`), 0o755))
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
}

// scheduleWakeupScenario emits a ScheduleWakeup tool_use with the given JSON input
// on the first turn, then a final result on the next turn. It varies its output by
// checking whether a ScheduleWakeup tool_use already exists in the session JSONL
// (the harness re-runs the script once per turn and requires advancing output).
//
// Reaching the second turn (the result frame) is itself the proof that ScheduleWakeup
// RESUMED: the script only emits the result once it sees its own prior tool_use in
// the session history, which can only happen because the runner re-ran it.
func scheduleWakeupScenario(input string) string {
	return `#!/bin/sh
seen=0
if [ -n "$A10N_MOCK_SESSION_FILE" ] && [ -f "$A10N_MOCK_SESSION_FILE" ]; then
  if grep -q '"name":"ScheduleWakeup"' "$A10N_MOCK_SESSION_FILE"; then
    seen=1
  fi
fi
if [ "$seen" -eq 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_wake","name":"ScheduleWakeup","input":` + input + `}]}}'
else
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
fi
`
}

// TestT012_01_ScheduleWakeupResumesScript: a valid ScheduleWakeup tool_use returns a
// success tool_result and RESUMES the turn (the runner re-runs the scenario), with NO
// compaction event and NO SessionStart source=compact. ScheduleWakeup is a plain
// resume tool — compaction is a separate, script-emitted event (see 013_compaction).
// sr:proves schedule-wakeup/claude
func TestT012_01_ScheduleWakeupResumesScript(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sources.txt")
	writeSessionStartHook(t, dir, logFile)

	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte(scheduleWakeupScenario(`{"delaySeconds":120,"reason":"more work","prompt":"resume work"}`)), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-wake", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "valid ScheduleWakeup run must succeed\noutput:\n%s", out)

	// The success tool_result mirrors the real harness text (so the task-executor's
	// result-text fallback still parses it).
	assert.Contains(t, out, "Next wakeup scheduled",
		"ScheduleWakeup success result must mirror real claude text\noutput:\n%s", out)

	// The script was RE-RUN (resume): it only emits the final result frame after it
	// sees its own prior ScheduleWakeup tool_use in the session history.
	assert.Contains(t, out, `"result":"done"`,
		"ScheduleWakeup must resume the turn (script re-run reaches the result frame)\noutput:\n%s", out)

	// ScheduleWakeup must NOT compact: no compaction event, no SessionStart compact.
	assert.NotContains(t, out, `"isCompactSummary":true`,
		"ScheduleWakeup must NOT emit a compaction event\noutput:\n%s", out)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "SessionStart hook must have fired")
	assert.Contains(t, string(data), "startup", "SessionStart should fire at startup")
	assert.NotContains(t, string(data), "compact",
		"ScheduleWakeup must NOT fire SessionStart source=compact\nsources:\n%s", string(data))
}

// TestT012_02_InvalidArgsProduceToolError: a ScheduleWakeup missing the required
// delaySeconds arg must produce an is_error tool_result (and, as for any tool, must
// not compact).
// sr:proves schedule-wakeup/claude
func TestT012_02_InvalidArgsProduceToolError(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sources.txt")
	writeSessionStartHook(t, dir, logFile)

	// Missing delaySeconds (only prompt provided).
	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte(scheduleWakeupScenario(`{"prompt":"resume"}`)), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-wake3", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "an invalid-args tool error must not crash the run\noutput:\n%s", out)

	// The tool_result is an error mentioning the missing arg.
	assert.Contains(t, out, `"is_error":true`,
		"invalid ScheduleWakeup args must surface an is_error tool_result\noutput:\n%s", out)
	assert.Contains(t, out, "delaySeconds",
		"the error should name the missing arg\noutput:\n%s", out)

	// No compaction on invalid args.
	assert.NotContains(t, out, `"isCompactSummary":true`,
		"invalid ScheduleWakeup must NOT compact\noutput:\n%s", out)
	data, _ := os.ReadFile(logFile)
	assert.NotContains(t, string(data), "compact",
		"invalid ScheduleWakeup must NOT fire SessionStart source=compact\nsources:\n%s", string(data))
}

// TestT012_03_NegativeDelayRejected: a negative delaySeconds is rejected as an error.
// sr:proves schedule-wakeup/claude
func TestT012_03_NegativeDelayRejected(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sources.txt")
	writeSessionStartHook(t, dir, logFile)

	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte(scheduleWakeupScenario(`{"delaySeconds":-5,"prompt":"resume"}`)), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-wake4", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)

	assert.Contains(t, out, `"is_error":true`, "negative delay must be rejected\noutput:\n%s", out)
	assert.NotContains(t, out, `"isCompactSummary":true`, "rejected ScheduleWakeup must not compact")
}
