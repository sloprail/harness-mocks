package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_07_SubagentBashSeesParentSessionID: a subagent's own Bash tool_use sees
// CLAUDE_CODE_SESSION_ID = the PARENT session id — the same id its SubagentStart/Stop
// hooks get (real claude shares one session_id across a session's subagents) — and
// NOT a value inherited from the mock's own environment (planted here as a decoy).
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID)
// sr:docs https://code.claude.com/docs/en/sub-agents
func TestT009_07_SubagentBashSeesParentSessionID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "decoy-outer-session")

	sidLog := filepath.Join(dir, "sub-sid.log")
	// Turn 1: a Bash tool_use recording CLAUDE_CODE_SESSION_ID. Turn 2 (the log
	// exists, so the Bash call ran): finish. The log file is the turn-to-turn state,
	// so the check cannot be confused by what the shared session file holds.
	subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
if [ -f "`+sidLog+`" ]; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_sub_bash","name":"Bash","input":{"command":"printf %s \"$CLAUDE_CODE_SESSION_ID\" > `+sidLog+`"}}]}}'
`)
	orch := writeScript(t, dir, "orch.sh", orchestratorNoIsolation(subScript))

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "sess-parent-sid", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	got, err := os.ReadFile(sidLog)
	require.NoError(t, err, "subagent's Bash tool_use must have run; output:\n%s", out)
	assert.Equal(t, "sess-parent-sid", string(got),
		"a subagent's Bash tool must see the parent session id, not the inherited decoy")
}
