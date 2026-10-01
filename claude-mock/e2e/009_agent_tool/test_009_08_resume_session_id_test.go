package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_08_ResumedRunChildrenSeeTheResumedID: on `--resume <session-id>`, the
// Bash tool and hook commands see the resumed session's id as
// CLAUDE_CODE_SESSION_ID, matching each other, and not a value inherited from
// the mock's own environment (planted as a decoy). The docs state it; no
// recorded run shows a resume yet.
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID: "On --resume <session-id> it receives the resumed ID, matching hooks and Bash")
// sr:proves subprocess-session-env/claude
func TestT009_08_ResumedRunChildrenSeeTheResumedID(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	bashLog := filepath.Join(dir, "bash-sid.log")
	hookLog := filepath.Join(dir, "hook-sid.log")
	writeSettings(t, dir, map[string]string{
		"Stop": writeHook(t, dir, "stop.sh", `cat >/dev/null
printf %s "$CLAUDE_CODE_SESSION_ID" > "`+hookLog+`"`),
	})
	// Turn 1: a Bash tool_use recording CLAUDE_CODE_SESSION_ID; turn 2 (it ran): finish.
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
if [ -f "`+bashLog+`" ]; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_bash","name":"Bash","input":{"command":"printf %s \"$CLAUDE_CODE_SESSION_ID\" > `+bashLog+`"}}]}}'
`)

	out, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-resumed", "--project-dir", dir, "--config-dir", configDir, "-p", "first")
	require.Equal(t, 0, code, "fresh run; output:\n%s", out)
	require.NoError(t, os.Remove(bashLog))
	require.NoError(t, os.Remove(hookLog))

	t.Setenv("CLAUDE_CODE_SESSION_ID", "decoy-outer-session")
	out, code = runInDir(t, dir, nil,
		"--script", script, "--resume", "s-resumed", "--project-dir", dir, "--config-dir", configDir, "-p", "second")
	require.Equal(t, 0, code, "resumed run; output:\n%s", out)

	bash, err := os.ReadFile(bashLog)
	require.NoError(t, err, "the resumed run's Bash tool_use must have run; output:\n%s", out)
	assert.Equal(t, "s-resumed", string(bash), "Bash sees the resumed id")
	hook, err := os.ReadFile(hookLog)
	require.NoError(t, err, "the resumed run's Stop hook must fire")
	assert.Equal(t, "s-resumed", string(hook), "hooks see the resumed id, matching Bash")
}
