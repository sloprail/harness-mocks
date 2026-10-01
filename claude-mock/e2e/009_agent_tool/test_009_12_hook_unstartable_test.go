package e2e

import (
	"path/filepath"
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
	assert.Equal(t, "Failed with non-blocking status code: /bin/sh: "+missing+": No such file or directory", errs[0]["stderr"])
}
