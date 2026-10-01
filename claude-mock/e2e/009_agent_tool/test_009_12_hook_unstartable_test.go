package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A hook whose command cannot start (the path does not exist, so the shell
// exits 127) is a non-blocking error like any other status: the tool runs, and
// the notice carries the interpreter's message (recorded:
// snapshots/runs/hook-unstartable).
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
// sr:proves hook-output-transcript-records/claude
func TestT009_12_UnstartableHookIsNonBlockingNotice(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	missing := filepath.Join(dir, "no-such-dir", "hook.sh")
	writeSettings(t, dir, map[string]string{"PreToolUse": missing})
	toolFile := filepath.Join(dir, "tool-ran")
	runsLog := filepath.Join(dir, "runs.log")
	script := toolScenario(t, dir, runsLog, filepath.Join(dir, "session.copy"), toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-127", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "the tool runs past a hook that cannot start")

	errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
	require.Len(t, errs, 1, "one notice for the hook that cannot start")
	assert.Equal(t, "PreToolUse:Bash", errs[0]["hookName"])
	assert.Equal(t, float64(127), errs[0]["exitCode"])
	// the interpreter's own message follows; its wording is the platform's shell
	// ("No such file or directory" on macOS, "not found" under dash on Linux)
	stderr, _ := errs[0]["stderr"].(string)
	assert.True(t, strings.HasPrefix(stderr, "Failed with non-blocking status code: /bin/sh: "), stderr)
	assert.Contains(t, stderr, missing)
}

// A non-blocking failure's notice keeps the hook's whole stderr in the
// transcript record, every line (recorded: snapshots/runs/hook-unstartable).
// The docs' "followed by the first line of stderr" describes what the notice
// shows on screen; the record the recording holds is the full text.
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_12_NonBlockingNoticeKeepsWholeStderr(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	writeSettings(t, dir, map[string]string{
		"PreToolUse": writeHook(t, dir, "two-lines.sh", "cat >/dev/null\nprintf 'first line of stderr\\nsecond line of stderr\\n' >&2\nexit 1"),
	})
	toolFile := filepath.Join(dir, "tool-ran")
	script := toolScenario(t, dir, filepath.Join(dir, "runs.log"), filepath.Join(dir, "session.copy"), toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-2l", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "exit 1 does not block the tool")

	errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
	require.Len(t, errs, 1)
	assert.Equal(t, "Failed with non-blocking status code: first line of stderr\nsecond line of stderr", errs[0]["stderr"])
}

// Stdout tried as JSON that does not parse is a non-blocking error on every
// exit status but 2, and the tool runs (recorded: snapshots/runs/hook-exit-json,
// echo j and echo k): JSON lines one of which sets an output field are such
// stdout, and on a non-zero status the hook's own stderr follows the message.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
// sr:proves hook-exit-code-semantics/claude
func TestT009_12_UnparseableJSONIsNonBlockingOnAnyStatusBut2(t *testing.T) {
	for _, tc := range []struct {
		name, stdout, stderr, wantTail string
		code                           int
	}{
		{"lines one of which sets a field, exit 0", `{"x": 1}` + "\n" + `{"decision": "block"}`, "", "", 0},
		{"malformed, exit 1", "{not json on exit 1}", "malformed on exit 1", "\n\nHook exited 1 with stderr:\nmalformed on exit 1", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			writeSettings(t, dir, map[string]string{"PreToolUse": hookWithRaw(t, dir, tc.stdout, tc.stderr, tc.code)})
			toolFile := filepath.Join(dir, "tool-ran")
			script := toolScenario(t, dir, filepath.Join(dir, "runs.log"), filepath.Join(dir, "session.copy"), toolFile)
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-uj", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, "output:\n%s", out)
			assert.True(t, fileExists(toolFile), "a non-blocking error lets the tool run")
			errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
			require.Len(t, errs, 1)
			stderr, _ := errs[0]["stderr"].(string)
			assert.True(t, strings.HasPrefix(stderr, "Hook output looks like a JSON object but is not valid JSON"), stderr)
			assert.True(t, strings.HasSuffix(stderr, tc.wantTail), stderr)
			assert.Equal(t, float64(tc.code), errs[0]["exitCode"])
		})
	}
}

// A UserPromptSubmit exit 2 whose JSON sets suppressOriginalPrompt gets a
// block message without the "Original prompt:" tail (recorded:
// snapshots/runs/prompt-blocked-suppressed); the run still ends successfully.
// sr:docs https://code.claude.com/docs/en/hooks#what-a-blocked-prompt-leaves-behind
// sr:proves hook-exit-code-semantics/claude
// sr:proves user-prompt-submit-hook/claude
func TestT009_12_BlockedPromptCanSuppressItsText(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]string{"UserPromptSubmit": hookWithRaw(t, dir,
		`{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","suppressOriginalPrompt":true}}`, "prompt refused", 2)})
	ran := filepath.Join(dir, "ran")
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\n: > \""+ran+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-sp", "--project-dir", dir, "-p", "the secret prompt")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.False(t, fileExists(ran), "a blocked prompt never reaches the model")
	assert.Contains(t, out, "UserPromptSubmit operation blocked by hook:")
	assert.Contains(t, out, "prompt refused")
	assert.NotContains(t, out, "Original prompt:")
	assert.NotContains(t, out, "the secret prompt")
}

// Several hooks on one matcher all run; each failure gets its own notice, in
// order (recorded: snapshots/runs/hook-unstartable, an unstartable hook then a
// failing one).
// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_12_EachFailingHookOnAMatcherGetsItsNotice(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	missing := filepath.Join(dir, "no-such-dir", "hook.sh")
	failing := writeHook(t, dir, "fails.sh", "cat >/dev/null\necho 'second hook failed' >&2\nexit 1")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(
		`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+missing+`"},{"type":"command","command":"`+failing+`"}]}]}}`), 0o644))
	toolFile := filepath.Join(dir, "tool-ran")
	script := toolScenario(t, dir, filepath.Join(dir, "runs.log"), filepath.Join(dir, "session.copy"), toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-2h", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "neither failure blocks the tool")
	errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
	require.Len(t, errs, 2, "one notice per failing hook")
	assert.Equal(t, missing, errs[0]["command"])
	assert.Equal(t, float64(127), errs[0]["exitCode"])
	assert.Equal(t, failing, errs[1]["command"])
	assert.Equal(t, "Failed with non-blocking status code: second hook failed", errs[1]["stderr"])
}
