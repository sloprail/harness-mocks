package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// WorktreeCreate and WorktreeRemove have no matcher support (docs, Matcher
// patterns): a hook under a matcher that matches nothing still fires for each.
// sr:proves hook-matcher-filter/claude
func TestT002_09_WorktreeHooksIgnoreTheirMatcher(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookScript(t, dir, logFile)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	entry := `[{"matcher":"matches-nothing","hooks":[{"type":"command","command":"` + hook + `"}]}]`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"WorktreeCreate":`+entry+`,"WorktreeRemove":`+entry+`}}`), 0o644))
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/a"}'
printf '%s\n' '{"type":"worktree_remove","worktree_path":"/x/.claude/worktrees/feat-a"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "wm-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"feat/a", "feat-a"}, strings.Fields(string(data)), "both hooks fired though neither matcher matched")
}
