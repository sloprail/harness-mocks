package e2e

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT011_04_ProjectAndPluginHooksRunTogether replays the scenario of
// snapshots/runs/plugin-hooks through the binary: a PreToolUse Bash hook of the
// project and of an enabled plugin both run for one call, a disabled plugin's
// and one from a marketplace the settings never declare do not, and a plugin's
// hook command, which names the project's script through ${CLAUDE_PROJECT_DIR},
// is told where the plugin is installed and where its data lives.
// sr:proves plugin-hooks/claude
func TestT011_04_ProjectAndPluginHooksRunTogether(t *testing.T) {
	dir := t.TempDir()
	mpRoot := t.TempDir()
	cache := t.TempDir()
	logFile := filepath.Join(dir, "ran.log")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
cat >/dev/null
echo "$1 root=$CLAUDE_PLUGIN_ROOT data=$CLAUDE_PLUGIN_DATA project=$CLAUDE_PROJECT_DIR" >> `+logFile+`
`), 0o755))

	require.NoError(t, os.MkdirAll(filepath.Join(mpRoot, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mpRoot, ".claude-plugin", "marketplace.json"), []byte(`{
  "name": "mk", "owner": {"name": "Test"},
  "plugins": [
    {"name": "p1", "source": "./plugins/p1", "description": "enabled"},
    {"name": "p2", "source": "./plugins/p2", "description": "disabled"}
  ]
}`), 0o644))
	for _, p := range []string{"p1", "p2"} {
		require.NoError(t, os.MkdirAll(filepath.Join(mpRoot, "plugins", p, "hooks"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(mpRoot, "plugins", p, "hooks", "hooks.json"), []byte(`{
  "hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "\"${CLAUDE_PROJECT_DIR}\"/hook.sh `+p+`"}]}]}
}`), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{
  "extraKnownMarketplaces": {"mk": {"source": {"source": "directory", "path": "`+mpRoot+`"}}},
  "enabledPlugins": {"p1@mk": true, "p2@mk": false, "p3@undeclared": true},
  "hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "\"$CLAUDE_PROJECT_DIR\"/hook.sh project"}]}]}
}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
F="$A10N_MOCK_SESSION_FILE"
if ! grep -q turn-a "$F" 2>/dev/null; then
cat <<'JSONL'
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"b1turn-a","name":"Bash","input":{"command":"true"}}]}}
JSONL
exit 0
fi
echo '{"type":"result","subtype":"success","result":"done"}'
`), 0o755))

	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_PLUGIN_CACHE_DIR=" + cache}, "--script", script,
		"--session-id", "ph-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "go")
	require.Equal(t, 0, code, out)

	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	sort.Strings(lines)
	require.Len(t, lines, 2, "the project's hook and the enabled plugin's, no more: %v", lines)
	assert.True(t, strings.HasPrefix(lines[0], "p1 root="+filepath.Join(mpRoot, "plugins", "p1")+" data="), lines[0])
	assert.Contains(t, lines[0], " project="+dir)
	assert.Equal(t, "project root= data= project="+dir, lines[1], "a project hook is told no plugin")
	dataDir := strings.TrimPrefix(strings.Fields(lines[0])[2], "data=")
	assert.NotEqual(t, filepath.Join(mpRoot, "plugins", "p1"), dataDir, "the plugin's data is kept apart from its installation")
	assert.NotContains(t, dataDir, mpRoot, "and never inside the marketplace the user keeps")
	st, err := os.Stat(dataDir)
	require.NoError(t, err)
	assert.True(t, st.IsDir())
}
