package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_03_SubagentWorktreeCwdIsolated pins the real-claude behaviour the mock
// must mirror: a subagent spawned with isolation="worktree" runs in a FRESH worktree
// at <parent-cwd>/.claude/worktrees/agent-<agentID>, so its observed cwd DIFFERS from
// the parent while the session_id stays SHARED.
//
// This is the identity that lets the a10n checks engine key session-scoped state by
// (cwd_hash, session_id): sub and root share the session but have distinct cwds, so a
// delegated subagent's per-cwd folders/contexts don't collide with the root's. If the
// mock reported the parent cwd for the subagent (the pre-fix behaviour), that isolation
// could never be exercised in a mock-based harness.
// sr:proves hook-common-payload/claude
// sr:proves subagent-worktree-isolation/claude
func TestT009_03_SubagentWorktreeCwdIsolated(t *testing.T) {
	dir := t.TempDir()
	// The mock now resolves symlinks before reporting/encoding cwd (matching real Claude
	// Code — verified empirically: its own PreToolUse payload `cwd` is already the resolved
	// form). On macOS t.TempDir() lives under /var, a symlink to /private/var, so the
	// reported cwd differs in SPELLING (not identity) from the raw dir string — resolve dir
	// the same way before comparing.
	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	startLog := filepath.Join(dir, "start.log")

	// SubagentStart hook records "<session_id>|<cwd>" from the payload.
	startHook := writeHook(t, dir, "start.sh",
		`input=$(cat)
sid=$(echo "$input" | grep -o '"session_id":"[^"]*"' | cut -d'"' -f4)
cwd=$(echo "$input" | grep -o '"cwd":"[^"]*"' | cut -d'"' -f4)
echo "$sid|$cwd" >> "`+startLog+`"`)
	writeSettings(t, dir, map[string]string{"SubagentStart": startHook})

	subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
`)
	// orchestratorScript emits isolation:"worktree" in the Agent tool input.
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", subScript))

	const sessionID = "sess-wt-1"
	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", sessionID, "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	data, err := os.ReadFile(startLog)
	require.NoError(t, err, "SubagentStart hook must fire")
	line := strings.TrimSpace(string(data))
	parts := strings.SplitN(line, "|", 2)
	require.Len(t, parts, 2, "start log must be <session_id>|<cwd>: %q", line)
	subSID, subCwd := parts[0], parts[1]

	// session_id is SHARED with the parent.
	assert.Equal(t, sessionID, subSID, "subagent shares the parent session_id")

	// cwd is the subagent's OWN worktree, not the parent dir.
	assert.NotEqual(t, resolvedDir, subCwd, "worktree subagent cwd must differ from parent cwd")
	assert.Contains(t, subCwd, filepath.Join(".claude", "worktrees", "agent-"),
		"worktree subagent cwd must be <parent>/.claude/worktrees/agent-<id>, got %q", subCwd)
	assert.True(t, strings.HasPrefix(subCwd, resolvedDir),
		"worktree cwd must be nested under the parent cwd %q, got %q", resolvedDir, subCwd)
}

// TestT009_03_SubagentNoIsolationSharesCwd is the OTHER axis value: without
// isolation="worktree" the subagent shares the parent cwd (session_id still shared).
// Mirrors the no_isolation subtest of the real-claude proof (test_009_04).
func TestT009_03_SubagentNoIsolationSharesCwd(t *testing.T) {
	dir := t.TempDir()
	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	startLog := filepath.Join(dir, "start.log")

	startHook := writeHook(t, dir, "start.sh",
		`input=$(cat)
sid=$(echo "$input" | grep -o '"session_id":"[^"]*"' | cut -d'"' -f4)
cwd=$(echo "$input" | grep -o '"cwd":"[^"]*"' | cut -d'"' -f4)
echo "$sid|$cwd" >> "`+startLog+`"`)
	writeSettings(t, dir, map[string]string{"SubagentStart": startHook})

	subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
`)
	// orchestratorNoIsolation emits an Agent tool_use WITHOUT an isolation field.
	orch := writeScript(t, dir, "orch.sh", orchestratorNoIsolation(subScript))

	const sessionID = "sess-noiso-1"
	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", sessionID, "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	data, err := os.ReadFile(startLog)
	require.NoError(t, err, "SubagentStart hook must fire")
	parts := strings.SplitN(strings.TrimSpace(string(data)), "|", 2)
	require.Len(t, parts, 2)
	subSID, subCwd := parts[0], parts[1]

	assert.Equal(t, sessionID, subSID, "subagent shares the parent session_id")
	assert.Equal(t, resolvedDir, subCwd, "no-isolation subagent shares the parent cwd")
}

// orchestratorNoIsolation spawns a subagent via an Agent tool_use with NO isolation
// field (unlike orchestratorScript, which sets isolation:"worktree").
func orchestratorNoIsolation(subScript string) string {
	return `#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_1","name":"Agent","input":{"description":"no-iso","prompt":"go","subagent_type":"general-purpose","script":"` + subScript + `"}}]}}'
`
}
