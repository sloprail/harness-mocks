package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spawningScript is a sub-agent scenario that, on its first turn, dispatches
// the sub-agent whose scenario is next (any path; it never runs when the call
// is refused), and once that call's result is in its transcript, finishes.
func spawningScript(next string) string {
	return fmt.Sprintf(`#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q -e 'agentId:' -e 'No such tool' "$A10N_MOCK_SESSION_FILE"; then
  printf '%%s\n' '{"type":"result","subtype":"success","result":"layer done","is_error":false}'
  exit 0
fi
printf '%%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_n","name":"Agent","input":{"description":"deeper","prompt":"go on","subagent_type":"general-purpose","script":"%s"}}]}}'
`, next)
}

// spawnLayers runs a main thread that dispatches a chain of sub-agents, each
// dispatching the next, and returns how many SubagentStart hooks fired, the
// the sub-agents' transcripts joined.
func spawnLayers(t *testing.T, layers int, env []string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	startLog := filepath.Join(dir, "start.log")
	writeSettings(t, dir, map[string]string{"SubagentStart": writeHook(t, dir, "start.sh",
		`cat >/dev/null; echo start >> "`+startLog+`"`)})
	next := "/nonexistent.sh"
	for i := layers; i >= 1; i-- {
		next = writeScript(t, dir, fmt.Sprintf("layer%d.sh", i), spawningScript(next))
	}
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", next))
	out, code := runInDir(t, dir, env, "--script", orch, "--session-id", "sess-limit", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, out)
	data, _ := os.ReadFile(startLog)
	files, err := filepath.Glob(filepath.Join(dir, ".claude-config", "projects", "*", "sess-limit", "subagents", "agent-*.jsonl"))
	require.NoError(t, err)
	var transcripts string
	for _, f := range files {
		b, _ := os.ReadFile(f)
		transcripts += string(b)
	}
	return len(nonEmptyLines(string(data))), transcripts
}

// TestT009_20_ThreeLayersByDefault: sub-agents nest three layers below the
// main conversation; the third has no Agent tool, so its dispatch is an error
// result and a fourth sub-agent never starts.
// sr:proves nested-subagents/claude
func TestT009_20_ThreeLayersByDefault(t *testing.T) {
	started, transcripts := spawnLayers(t, 5, nil)
	assert.Equal(t, 3, started, "three layers of sub-agents start, no fourth")
	assert.Contains(t, transcripts, "No such tool available: Agent")
	assert.Contains(t, transcripts, `"is_error":true`)
}

// TestT009_21_SpawnDepthOverride: CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH sets the
// number of layers; 2 lets sub-agents delegate once more and no further, and 1
// turns nesting off.
// sr:proves nested-subagents/claude
func TestT009_21_SpawnDepthOverride(t *testing.T) {
	started, _ := spawnLayers(t, 5, []string{"CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=2"})
	assert.Equal(t, 2, started)
	started, _ = spawnLayers(t, 5, []string{"CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1"})
	assert.Equal(t, 1, started, "a limit of 1 turns nesting off")
	started, _ = spawnLayers(t, 5, []string{"CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=4"})
	assert.Equal(t, 4, started, "a higher limit lets nesting go deeper")
}
