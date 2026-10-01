package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopOnce is a Stop hook that answers with stdout/stderr/code the first time
// it fires and exits 0 after, so a block re-runs the model once.
func stopOnce(t *testing.T, dir, stdout, stderr string, code int) string {
	once := filepath.Join(dir, "stopped-once")
	body := "cat >/dev/null\nif [ ! -f \"" + once + "\" ]; then : > \"" + once + "\"\n"
	if stdout != "" {
		body += "printf '%s' '" + stdout + "'\n"
	}
	if stderr != "" {
		body += "echo '" + stderr + "' >&2\n"
	}
	return writeHook(t, dir, "stop.sh", body+"exit "+string(rune('0'+code))+"\nfi\nexit 0")
}

// runStopScenario runs a one-turn scenario under a Stop hook and returns how
// many times the model ran and what its session file held.
func runStopScenario(t *testing.T, dir, stopHook string) (int, string) {
	runs, sess := filepath.Join(dir, "runs"), filepath.Join(dir, "sess")
	writeSettings(t, dir, map[string]string{"Stop": stopHook})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\necho run >> \""+runs+"\"\ncp \"$A10N_MOCK_SESSION_FILE\" \""+sess+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-cls", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	return strings.Count(readOrEmpty(runs), "run"), readOrEmpty(sess)
}

// A SessionEnd hook that fails is shown to the user only: the harness writes
// "SessionEnd hook [<command>] failed: <stderr>" to its own stderr and the run
// ends normally (recorded: snapshots/runs/hook-exit-codes, stderr.txt).
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_13_SessionEndFailureIsShownToTheUser(t *testing.T) {
	dir := t.TempDir()
	hook := writeHook(t, dir, "end.sh", "cat >/dev/null\necho 'session-end stderr on exit 1' >&2\nexit 1")
	writeSettings(t, dir, map[string]string{"SessionEnd": hook})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-se", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Contains(t, out, "SessionEnd hook ["+hook+"] failed: session-end stderr on exit 1")
}

// Any status other than 0 and 2 is a non-blocking error, not only 1: an exit 3
// lets the tool run and leaves the notice with its status.
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_13_Exit3IsNonBlocking(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	writeSettings(t, dir, map[string]string{"PreToolUse": hookWithRaw(t, dir, "", "three", 3)})
	toolFile := filepath.Join(dir, "tool-ran")
	script := toolScenario(t, dir, filepath.Join(dir, "runs.log"), filepath.Join(dir, "session.copy"), toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-e3", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile))
	errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
	require.Len(t, errs, 1)
	assert.Equal(t, float64(3), errs[0]["exitCode"])
	assert.Equal(t, "Failed with non-blocking status code: three", errs[0]["stderr"])
}

// A hook that cannot start is a non-blocking error on any event, not only a
// tool's: on UserPromptSubmit the prompt still reaches the model.
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_13_UnstartableHookOnUserPromptSubmit(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	missing := filepath.Join(dir, "no-such-dir", "hook.sh")
	writeSettings(t, dir, map[string]string{"UserPromptSubmit": missing})
	ran := filepath.Join(dir, "ran")
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\n: > \""+ran+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-u127", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(ran), "the prompt reaches the model")
	errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
	require.Len(t, errs, 1)
	assert.Equal(t, "UserPromptSubmit", errs[0]["hookName"])
	assert.Equal(t, float64(127), errs[0]["exitCode"])
}

// A block's message is the JSON's blocking reason when the hook gives one, the
// stderr otherwise, on every event that blocks: a Stop exit 2 with a JSON
// block reason feeds that reason back, not the stderr.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2
// sr:proves hook-exit-code-semantics/claude
func TestT009_13_StopBlockMessageIsTheJSONReason(t *testing.T) {
	dir := t.TempDir()
	runs, sess := runStopScenario(t, dir, stopOnce(t, dir, `{"decision": "block", "reason": "the JSON stop reason"}`, "the stderr loses", 2))
	assert.Equal(t, 2, runs, "exit 2 blocks the stop")
	assert.Contains(t, sess, "the JSON stop reason")
	assert.NotContains(t, sess, "the stderr loses")
}

// On a non-blocking status, valid JSON decides instead of the status: a Stop
// hook exiting 1 with {"decision":"block"} blocks the stop, with no
// non-blocking notice.
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_13_Exit1JSONDecidesOnStop(t *testing.T) {
	dir := t.TempDir()
	runs, sess := runStopScenario(t, dir, stopOnce(t, dir, `{"decision": "block", "reason": "json decides on exit 1"}`, "", 1))
	assert.Equal(t, 2, runs, "the JSON's block holds on exit 1")
	assert.Contains(t, sess, "json decides on exit 1")
	assert.NotContains(t, sess, "Failed with non-blocking status code")
}

// Plain-text stdout does not change a non-zero, non-2 status: the hook is a
// non-blocking error, the tool runs, and the notice carries its stderr.
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_13_PlainTextStdoutOnExit1IsNonBlocking(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	writeSettings(t, dir, map[string]string{"PreToolUse": hookWithRaw(t, dir, "just some words", "plain on exit 1", 1)})
	toolFile := filepath.Join(dir, "tool-ran")
	script := toolScenario(t, dir, filepath.Join(dir, "runs.log"), filepath.Join(dir, "session.copy"), toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-pt1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "the tool runs")
	errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
	require.Len(t, errs, 1)
	assert.Equal(t, float64(1), errs[0]["exitCode"])
	assert.Equal(t, "Failed with non-blocking status code: plain on exit 1", errs[0]["stderr"])
	assert.Empty(t, attachmentsOf(allRecords(t, cfg), "hook_blocking_error"))
}

// SessionEnd cannot block: an exit 2 is shown to the user only, like any other
// failure, and the run ends normally.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_13_SessionEndExit2IsShownToTheUserOnly(t *testing.T) {
	dir := t.TempDir()
	hook := writeHook(t, dir, "end.sh", "cat >/dev/null\necho 'session-end stderr on exit 2' >&2\nexit 2")
	writeSettings(t, dir, map[string]string{"SessionEnd": hook})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-se2", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Contains(t, out, "SessionEnd hook ["+hook+"] failed: session-end stderr on exit 2")
}
