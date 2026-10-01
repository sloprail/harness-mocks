package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookScript writes a helper hook.sh that appends worktree_name to logFile.
func hookScript(t *testing.T, dir, logFile string) string {
	t.Helper()
	p := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(p, []byte(`#!/bin/sh
input=$(cat)
name=$(echo "$input" | grep -o '"worktree_name":"[^"]*"' | cut -d'"' -f4)
echo "$name" >> "`+logFile+`"
`), 0o755))
	return p
}

func writeSettings(t *testing.T, dir, event, hookPath string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"` + event + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hookPath + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
}

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o755))
	return p
}

// --- WorktreeCreate ---

// TestT002_01_WorktreeCreateHookFires: positive — hook fires and receives worktree_name.
// staged:proves worktree-hooks/claude
func TestT002_01_WorktreeCreateHookFires(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	writeSettings(t, dir, "WorktreeCreate", hookScript(t, dir, logFile))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/my-feature"}'
printf '%s\n' '{"type":"system","subtype":"init","session_id":"s1","tools":[]}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "hook must have fired and written to log")
	assert.Contains(t, string(data), "feat/my-feature")
}

// TestT002_02_WorktreeCreateNotForwardedToStdout: control record must be consumed.
func TestT002_02_WorktreeCreateNotForwardedToStdout(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/x"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	assert.NotContains(t, out, "worktree_create", "control record must not appear in stdout")
}

// TestT002_03_WorktreeCreateBlockCausesNonZeroExit: hook exit 2 must block the run.
// staged:proves worktree-hooks/claude
func TestT002_03_WorktreeCreateBlockCausesNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	blockHook := writeScript(t, dir, "block.sh", `#!/bin/sh
echo "blocked" >&2
exit 2
`)
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"WorktreeCreate":[{"matcher":"*","hooks":[{"type":"command","command":"` + blockHook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/blocked"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"should not reach","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code, "blocking hook must cause non-zero exit")
}

// TestT002_04_WorktreeCreateFailsOnAnyNonZeroExit: unlike most events, where
// only exit 2 blocks, any non-zero exit of a WorktreeCreate hook fails the
// worktree's creation (docs, "Exit code 2 behavior per event").
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT002_04_WorktreeCreateFailsOnAnyNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	warnHook := writeScript(t, dir, "warn.sh", `#!/bin/sh
echo "warning" >&2
exit 1
`)
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"WorktreeCreate":[{"matcher":"*","hooks":[{"type":"command","command":"` + warnHook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/warn"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"ok","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code, "an exit 1 WorktreeCreate hook fails the creation")
}

// --- WorktreeRemove ---

// TestT002_05_WorktreeRemoveHookFires: positive — WorktreeRemove hook fires.
// staged:proves worktree-hooks/claude
func TestT002_05_WorktreeRemoveHookFires(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	writeSettings(t, dir, "WorktreeRemove", hookScript(t, dir, logFile))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_remove","worktree_name":"feat/old"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "WorktreeRemove hook must fire")
	assert.Contains(t, string(data), "feat/old")
}

// TestT002_05b_WorktreeRemoveFailsOnAnyNonZeroExit: any non-zero exit of a
// WorktreeRemove hook fails the removal (docs, "Exit code 2 behavior per event").
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT002_05b_WorktreeRemoveFailsOnAnyNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	warnHook := writeScript(t, dir, "warn.sh", `#!/bin/sh
echo "warning" >&2
exit 1
`)
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"WorktreeRemove":[{"matcher":"*","hooks":[{"type":"command","command":"` + warnHook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_remove","worktree_name":"feat/old"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"ok","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code, "an exit 1 WorktreeRemove hook fails the removal")
}

// TestT002_06_WorktreeRemoveBlockCausesNonZeroExit: WorktreeRemove exit 2 blocks.
func TestT002_06_WorktreeRemoveBlockCausesNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	blockHook := writeScript(t, dir, "block.sh", "#!/bin/sh\nexit 2\n")
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"WorktreeRemove":[{"matcher":"*","hooks":[{"type":"command","command":"` + blockHook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_remove","worktree_name":"feat/blocked"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"ok","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code)
}

// TestT002_07_BothWorktreeEventsDistinct: WorktreeCreate and WorktreeRemove fire separately.
// staged:proves worktree-hooks/claude
func TestT002_07_BothWorktreeEventsDistinct(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	fullHook := writeScript(t, dir, "hook.sh", `#!/bin/sh
input=$(cat)
evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
name=$(echo "$input" | grep -o '"worktree_name":"[^"]*"' | cut -d'"' -f4)
echo "$evt:$name" >> "`+logFile+`"
`)
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{
  "WorktreeCreate":[{"matcher":"*","hooks":[{"type":"command","command":"` + fullHook + `"}]}],
  "WorktreeRemove":[{"matcher":"*","hooks":[{"type":"command","command":"` + fullHook + `"}]}]
}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/new"}'
printf '%s\n' '{"type":"worktree_remove","worktree_name":"feat/old"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2, "two hooks must fire")
	assert.Contains(t, string(data), "WorktreeCreate:feat/new")
	assert.Contains(t, string(data), "WorktreeRemove:feat/old")
}
