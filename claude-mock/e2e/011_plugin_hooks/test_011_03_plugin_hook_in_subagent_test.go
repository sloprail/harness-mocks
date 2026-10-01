package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT011_05_PluginHookRunsInsideASubagent: plugin hooks also run inside
// sub-agents, and their payload names the sub-agent: a plugin's PreToolUse hook
// fires for the Bash call a sub-agent makes, with agent_id and agent_type, and
// not for the parent's own Agent call as a sub-agent's (docs, Hook locations;
// Common input fields).
// sr:proves plugin-hooks/claude
func TestT011_05_PluginHookRunsInsideASubagent(t *testing.T) {
	dir, mpRoot, cache := t.TempDir(), t.TempDir(), t.TempDir()
	logFile := filepath.Join(dir, "payloads.log")

	require.NoError(t, os.MkdirAll(filepath.Join(mpRoot, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mpRoot, ".claude-plugin", "marketplace.json"), []byte(`{"name":"mp"}`), 0o644))
	pluginDir := filepath.Join(mpRoot, "plugins", "pre")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "hooks", "log.sh"),
		[]byte("#!/bin/sh\ncat >> \""+logFile+"\"\necho >> \""+logFile+"\"\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "hooks", "hooks.json"), []byte(`{
  "hooks": {"PreToolUse": [{"matcher":"*","hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/log.sh\""}]}]}
}`), 0o644))
	writeProjectSettings(t, dir, `{
  "enabledPlugins": {"pre@mp": true},
  "extraKnownMarketplaces": {"mp": {"source": {"source": "directory", "path": "`+mpRoot+`"}}}
}`)

	// The sub-agent makes one Bash call, then finishes.
	sub := filepath.Join(dir, "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(`#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q tool_result "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_sub_bash","name":"Bash","input":{"command":"echo from-the-subagent"}}]}}'
`), 0o755))
	// The parent dispatches it, and ends once the dispatch has answered.
	root := filepath.Join(dir, "root.sh")
	require.NoError(t, os.WriteFile(root, []byte(`#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_1","name":"Agent","input":{"description":"helper","prompt":"go","subagent_type":"general-purpose","script":"`+sub+`"}}]}}'
`), 0o755))

	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_PLUGIN_CACHE_DIR=" + cache},
		"--script", root, "--plugin-cache-dir", cache, "--session-id", "pl-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code, out)

	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "the plugin's PreToolUse hook fired")
	byTool := map[string]map[string]any{}
	for _, l := range strings.Split(string(data), "\n") {
		var p map[string]any
		if json.Unmarshal([]byte(l), &p) == nil {
			byTool[p["tool_name"].(string)] = p
		}
	}
	require.Contains(t, byTool, "Bash", "the plugin hook ran for the sub-agent's Bash call: %s", data)
	bash := byTool["Bash"]
	assert.NotEmpty(t, bash["agent_id"], "the sub-agent's payload names it")
	assert.Equal(t, "general-purpose", bash["agent_type"])
	if agent, ok := byTool["Agent"]; ok {
		assert.Empty(t, agent["agent_id"], "the parent's own call is not a sub-agent's")
	}
}
