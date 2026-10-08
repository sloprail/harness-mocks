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

// A plugin's hook is a copy of its own, kept apart from the project's even when
// the command is the same: the command runs once for the project's entry and once
// for the plugin's, which alone is told the plugin's root (recorded in
// snapshots/runs/plugin-hooks-same-command: two p1 runs).
// sr:proves plugin-hooks/claude
func TestT011_04_ASamePluginAndProjectHookRunsTwice(t *testing.T) {
	logs, err := filepath.Glob(filepath.Join("..", "..", "snapshots", "runs", "plugin-hooks-same-command", "samples", "*", "payloads.jsonl"))
	require.NoError(t, err)
	require.Len(t, logs, 1)
	rec, err := os.ReadFile(logs[0])
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(rec), `"hook_ran":"p1"`), "recorded: the same command ran for the project and for the plugin")

	dir, mpRoot, cache := t.TempDir(), t.TempDir(), t.TempDir()
	logFile := filepath.Join(dir, "ran.log")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\necho \"$1 root=$CLAUDE_PLUGIN_ROOT\" >> "+logFile+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(mpRoot, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mpRoot, ".claude-plugin", "marketplace.json"), []byte(`{"name":"mk","owner":{"name":"T"},"plugins":[{"name":"p1","source":"./plugins/p1","description":"d"}]}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(mpRoot, "plugins", "p1", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mpRoot, "plugins", "p1", "hooks", "hooks.json"), []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"\"${CLAUDE_PROJECT_DIR}\"/hook.sh p1"}]}]}}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{
  "extraKnownMarketplaces": {"mk": {"source": {"source": "directory", "path": "`+mpRoot+`"}}},
  "enabledPlugins": {"p1@mk": true},
  "hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "\"$CLAUDE_PROJECT_DIR\"/hook.sh p1"}]}]}
}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if ! grep -q turn-a "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"b1turn-a","name":"Bash","input":{"command":"true"}}]}}'
exit 0
fi
echo '{"type":"result","subtype":"success","result":"done"}'
`), 0o755))
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_PLUGIN_CACHE_DIR=" + cache}, "--script", script, "--session-id", "same-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "go")
	require.Equal(t, 0, code, out)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	sort.Strings(lines)
	require.Len(t, lines, 2, "the project's copy and the plugin's: %v", lines)
	assert.Equal(t, "p1 root=", lines[0], "the project's copy is told no plugin")
	assert.Equal(t, "p1 root="+filepath.Join(mpRoot, "plugins", "p1"), lines[1], "the plugin's copy is told its root")
}
