package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSettings writes a .claude/settings.json wiring one command hook per event.
// events maps an event name to the hook script path that should fire for it.
func writeSettings(t *testing.T, dir string, events map[string]string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	body := `{"hooks":{`
	first := true
	for evt, hookPath := range events {
		if !first {
			body += ","
		}
		first = false
		body += `"` + evt + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hookPath + `"}]}]`
	}
	body += `}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(body), 0o644))
}

// writeHook writes an executable hook script and returns its path.
func writeHook(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return p
}

// writeScript writes an executable scenario script and returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o755))
	return p
}

// orchestratorScript emits an Agent (or Task) tool_use spawning the subagent
// scenario at subScript, then emits a result frame once the synthesised
// tool_result (carrying `agentId:`) appears in the session file. The session
// file is the mock's turn-to-turn memory ($A10N_MOCK_SESSION_FILE).
func orchestratorScript(toolName, subScript string) string {
	return fmt.Sprintf(`#!/bin/sh
# On the second turn the session file already contains the synthesised
# tool_result with the literal "agentId:" — emit the final result then.
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
# First turn: spawn the subagent via the %[1]s tool.
printf '%%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_1","name":"%[1]s","input":{"description":"Implement task 2","prompt":"run a10n-task-executor workspace ensure --task-id 2","subagent_type":"general-purpose","isolation":"worktree","script":"%[2]s"}}]}}'
`, toolName, subScript)
}

// TestT009_01_AgentToolSpawnsSubagent: an Agent tool_use spawns a nested subagent.
// SubagentStart fires with agent_type AND a non-empty agent_id; the subagent
// scenario actually runs (writes a side-effect file); SubagentStop fires with the
// SAME agent_id; the Agent tool_result on stdout contains "agentId:".
// staged:proves subagent-lifecycle-hooks/claude
func TestT009_01_AgentToolSpawnsSubagent(t *testing.T) {
	dir := t.TempDir()
	startLog := filepath.Join(dir, "start.log")
	stopLog := filepath.Join(dir, "stop.log")
	sideEffect := filepath.Join(dir, "subagent-ran.txt")

	startHook := writeHook(t, dir, "start.sh",
		`input=$(cat)
agent=$(echo "$input" | grep -o '"agent_type":"[^"]*"' | cut -d'"' -f4)
id=$(echo "$input" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4)
echo "$agent:$id" >> "`+startLog+`"`)
	stopHook := writeHook(t, dir, "stop.sh",
		`input=$(cat)
id=$(echo "$input" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4)
echo "$id" >> "`+stopLog+`"`)
	writeSettings(t, dir, map[string]string{
		"SubagentStart": startHook,
		"SubagentStop":  stopHook,
	})

	subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
echo "ran" > "`+sideEffect+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"subagent done","is_error":false}'
`)
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", subScript))

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "sess-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// SubagentStart fired with agent_type and a non-empty agent_id.
	startData, err := os.ReadFile(startLog)
	require.NoError(t, err, "SubagentStart hook must fire")
	startLine := string(startData)
	assert.Contains(t, startLine, "general-purpose:", "SubagentStart must carry agent_type")
	agentID := extractAgentID(t, startLine)
	assert.NotEmpty(t, agentID, "SubagentStart agent_id must be non-empty")

	// The subagent scenario actually ran.
	_, err = os.ReadFile(sideEffect)
	require.NoError(t, err, "subagent scenario must have run (side-effect file missing)")

	// SubagentStop fired with the SAME agent_id.
	stopData, err := os.ReadFile(stopLog)
	require.NoError(t, err, "SubagentStop hook must fire")
	assert.Contains(t, string(stopData), agentID, "SubagentStop must carry the same agent_id as SubagentStart")

	// The Agent tool_result reached parent stdout and carries agentId:.
	assert.Contains(t, out, "agentId:", "Agent tool_result content must include agentId:")
}

// TestT009_02_TaskAliasWorksLikeAgent: the Task alias behaves identically to Agent.
func TestT009_02_TaskAliasWorksLikeAgent(t *testing.T) {
	dir := t.TempDir()
	startLog := filepath.Join(dir, "start.log")
	sideEffect := filepath.Join(dir, "subagent-ran.txt")

	startHook := writeHook(t, dir, "start.sh",
		`input=$(cat)
id=$(echo "$input" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4)
echo "$id" >> "`+startLog+`"`)
	writeSettings(t, dir, map[string]string{"SubagentStart": startHook})

	subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
echo "ran" > "`+sideEffect+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"task done","is_error":false}'
`)
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Task", subScript))

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "sess-task", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	startData, err := os.ReadFile(startLog)
	require.NoError(t, err, "SubagentStart hook must fire for the Task alias")
	assert.NotEmpty(t, extractAgentID(t, string(startData)), "Task alias must generate an agent_id")
	_, err = os.ReadFile(sideEffect)
	require.NoError(t, err, "Task alias subagent scenario must run")
	assert.Contains(t, out, "agentId:", "Task tool_result content must include agentId:")
}

// TestT009_03_TwoLevelNesting: the subagent's OWN script also emits an Agent
// tool_use, spawning a sub-subagent. Prove the second-level SubagentStart fires
// (a distinct agent_id) and the sub-subagent's side-effect file is written.
// staged:proves nested-subagents/claude
func TestT009_03_TwoLevelNesting(t *testing.T) {
	dir := t.TempDir()
	startLog := filepath.Join(dir, "start.log")
	level1Effect := filepath.Join(dir, "level1-ran.txt")
	level2Effect := filepath.Join(dir, "level2-ran.txt")

	startHook := writeHook(t, dir, "start.sh",
		`input=$(cat)
id=$(echo "$input" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4)
echo "$id" >> "`+startLog+`"`)
	writeSettings(t, dir, map[string]string{"SubagentStart": startHook})

	// Level-2 (deepest) subagent: just writes its side-effect file and finishes.
	subSub := writeScript(t, dir, "subsub.sh", `#!/bin/sh
echo "ran" > "`+level2Effect+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"level2 done","is_error":false}'
`)

	// Level-1 subagent: writes its own side-effect file, then on the first turn
	// spawns the level-2 subagent via an Agent tool_use, and on the second turn
	// (once the agentId: tool_result is in session) emits its result.
	subScript := writeScript(t, dir, "sub.sh", fmt.Sprintf(`#!/bin/sh
echo "ran" > "%[2]s"
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%%s\n' '{"type":"result","subtype":"success","result":"level1 done","is_error":false}'
  exit 0
fi
printf '%%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_2","name":"Agent","input":{"description":"nested","prompt":"deep","subagent_type":"general-purpose","script":"%[1]s"}}]}}'
`, subSub, level1Effect))

	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", subScript))

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "sess-nest", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// Both nesting levels ran.
	_, err := os.ReadFile(level1Effect)
	require.NoError(t, err, "level-1 subagent must run")
	_, err = os.ReadFile(level2Effect)
	require.NoError(t, err, "level-2 (sub-sub) subagent must run")

	// SubagentStart fired twice with two distinct agent_ids (one per level).
	startData, err := os.ReadFile(startLog)
	require.NoError(t, err, "SubagentStart hook must fire")
	ids := nonEmptyLines(string(startData))
	require.GreaterOrEqual(t, len(ids), 2, "SubagentStart must fire once per nesting level; got %q", ids)
	assert.NotEqual(t, ids[0], ids[1], "each nesting level must get a distinct agent_id")
}

// TestT009_04_NoSubagentScriptDegradesGracefully: an Agent tool_use without a
// script (and no env fallback) yields a non-error result mentioning the no-op.
func TestT009_04_NoSubagentScriptDegradesGracefully(t *testing.T) {
	dir := t.TempDir()

	orch := writeScript(t, dir, "orch.sh", `#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_3","name":"Agent","input":{"description":"noop","prompt":"nothing","subagent_type":"general-purpose"}}]}}'
`)

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "sess-noop", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0 even with no subagent script; output:\n%s", out)
	assert.Contains(t, out, "agentId:", "result must still carry agentId:")
	assert.Contains(t, out, "no subagent script", "missing script must degrade to a no-op note")
}

// extractAgentID pulls the trailing hex id from a "agent_type:agentid" or "agentid"
// log line (the first non-empty line). It tolerates the leading "agent_type:" form.
func extractAgentID(t *testing.T, logContent string) string {
	t.Helper()
	lines := nonEmptyLines(logContent)
	require.NotEmpty(t, lines, "log must have at least one line")
	first := lines[0]
	// Form may be "agent_type:id" or just "id". Take the part after the last ':'.
	for i := len(first) - 1; i >= 0; i-- {
		if first[i] == ':' {
			return first[i+1:]
		}
	}
	return first
}

// nonEmptyLines splits on newlines and drops empty lines.
func nonEmptyLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			if line != "" {
				out = append(out, line)
			}
			start = i + 1
		}
	}
	return out
}
