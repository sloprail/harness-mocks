package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_06_HookEnvCarriesClaudeCodeVars proves every hook command, at the
// root and in a dispatched sub-agent, sees this run's identity even when the
// mock itself runs inside another session, as the real harness does
// (runs/nested-session-env: claude launched over decoys for all six
// variables). CLAUDECODE, CLAUDE_CODE_CHILD_SESSION, CLAUDE_CODE_SESSION_ATTENDED,
// CLAUDE_PID and CLAUDE_CODE_SESSION_ID are this run's whatever was inherited;
// CLAUDE_CODE_ENTRYPOINT is the launcher's when it set one, and sdk-cli when
// not (runs/subprocess-session-env). A tool a hook shells to keys "am I under a
// harness" off these (sr-agent refuses with ErrNoHarness without them). Each
// hook's CLAUDE_CODE_SESSION_ID is the session_id of its own payload, as both
// runs record.
// sr:proves subprocess-session-env/claude
func TestT009_06_HookEnvCarriesClaudeCodeVars(t *testing.T) {
	for _, tc := range []struct {
		name, inheritedEntrypoint, wantEntrypoint string
	}{
		{"launcher declares the entrypoint", "decoy-launcher", "decoy-launcher"},
		{"no entrypoint inherited", "", "sdk-cli"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			events := []string{"SessionStart", "PreToolUse", "PostToolUse", "Stop", "SubagentStart"}
			hooks := map[string]string{}
			for _, ev := range events {
				hooks[ev] = writeHook(t, dir, ev+".sh", `cat > "`+filepath.Join(dir, ev+".payload")+`"
{ echo "CLAUDECODE=$CLAUDECODE"; echo "CLAUDE_CODE_ENTRYPOINT=$CLAUDE_CODE_ENTRYPOINT"; echo "CLAUDE_CODE_SESSION_ID=$CLAUDE_CODE_SESSION_ID"; echo "CLAUDE_CODE_CHILD_SESSION=$CLAUDE_CODE_CHILD_SESSION"; echo "CLAUDE_CODE_SESSION_ATTENDED=$CLAUDE_CODE_SESSION_ATTENDED"; echo "CLAUDE_PID=$CLAUDE_PID"; } > "`+filepath.Join(dir, ev+".env")+`"`)
			}
			writeSettings(t, dir, hooks)

			subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"subagent done","is_error":false}'
`)
			orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", subScript))

			// The mock runs as if nested in an outer session: every variable is
			// a decoy, so a value a hook sees is the one the MOCK set.
			out, code := runInDir(t, dir, []string{
				"CLAUDECODE=decoy", "CLAUDE_CODE_ENTRYPOINT=" + tc.inheritedEntrypoint,
				"CLAUDE_CODE_SESSION_ID=decoy-outer-session", "CLAUDE_CODE_CHILD_SESSION=decoy",
				"CLAUDE_CODE_SESSION_ATTENDED=decoy", "CLAUDE_PID=decoy",
			}, "--script", orch, "--session-id", "sess-envtest", "--project-dir", dir, "-p", "go")
			require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

			for _, ev := range events {
				data, err := os.ReadFile(filepath.Join(dir, ev+".env"))
				require.NoError(t, err, "%s hook must fire", ev)
				got := string(data)
				assert.Contains(t, got, "CLAUDECODE=1\n", ev)
				assert.Contains(t, got, "CLAUDE_CODE_ENTRYPOINT="+tc.wantEntrypoint+"\n", ev)
				assert.Contains(t, got, "CLAUDE_CODE_SESSION_ID=sess-envtest\n", ev)
				assert.Contains(t, got, "CLAUDE_CODE_CHILD_SESSION=1\n", ev)
				assert.Contains(t, got, "CLAUDE_CODE_SESSION_ATTENDED=0\n", ev)
				assert.Regexp(t, `(?m)^CLAUDE_PID=[0-9]+$`, got, ev)

				// the env's session id is the one the hook's own payload names
				raw, err := os.ReadFile(filepath.Join(dir, ev+".payload"))
				require.NoError(t, err, "%s hook payload", ev)
				var payload struct {
					SessionID string `json:"session_id"`
				}
				require.NoError(t, json.Unmarshal(raw, &payload), ev)
				assert.Equal(t, "sess-envtest", payload.SessionID, "%s payload session_id matches CLAUDE_CODE_SESSION_ID", ev)
			}
		})
	}
}
