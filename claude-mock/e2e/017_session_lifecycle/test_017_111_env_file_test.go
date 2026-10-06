package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_111_ASessionStartHookPersistsEnvironment: what a SessionStart hook appends to its
// CLAUDE_ENV_FILE is in effect for the session's Bash commands; the hooks of
// other events are given no such file (recorded: runs/env-file-persist).
// sr:proves subprocess-session-env/claude
func TestT017_111_ASessionStartHookPersistsEnvironment(t *testing.T) {
	dir := t.TempDir()
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
cat >/dev/null
if [ -n "$CLAUDE_ENV_FILE" ]; then echo "$CLAUDE_ENV_FILE" >>`+filepath.Join(dir, "files")+`; fi
[ -n "$CLAUDE_ENV_FILE" ] && printf 'export FROM_FILE=persisted\nexport FROM_PWD=$PWD/x\n' >>"$CLAUDE_ENV_FILE"
exit 0
`), 0o755))
	settings(t, dir, map[string]string{"SessionStart": hook, "PreToolUse": hook})
	calls := []string{
		toolUse("b1", "Bash", `{"command":"echo A=$FROM_FILE B=$FROM_PWD"}`),
		toolUse("b2", "Bash", `{"command":"echo BG=$FROM_FILE","run_in_background":true}`),
	}
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", calls...), "--session-id", "env-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code, out)
	cwd, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Contains(t, out, "A=persisted B="+cwd+"/x")
	files, err := os.ReadFile(filepath.Join(dir, "files"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "cfg", "session-env", "env-1", "sessionstart-hook-0.sh")+"\n", string(files), "only the SessionStart hook is given the file")
}
