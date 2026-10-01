package e2e

import (
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
