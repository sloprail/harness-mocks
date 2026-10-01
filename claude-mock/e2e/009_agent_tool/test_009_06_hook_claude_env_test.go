package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_06_HookEnvCarriesClaudeCodeVars proves the mock presents the three
// Claude-Code environment variables the real CLI sets on every session — CLAUDECODE=1,
// CLAUDE_CODE_ENTRYPOINT=sdk-cli, and CLAUDE_CODE_SESSION_ID — to BOTH the root hook path
// (Stop) AND the sub-agent hook path (SubagentStart, fired by the Agent-tool layer).
//
// This is the integration-level counterpart to the unit test in internal/hooks: a
// tool a hook shells to that detects "am I under a harness" (e.g. sr-agent, which
// REFUSES with ErrNoHarness when neither CLAUDECODE nor CLAUDE_CODE_ENTRYPOINT is set)
// must find that env whether the hook fired at the root or inside a dispatched
// sub-agent. Both paths run through the same invoker, so both must see all three.
// sr:proves subprocess-session-env/claude
func TestT009_06_HookEnvCarriesClaudeCodeVars(t *testing.T) {
	dir := t.TempDir()
	rootEnv := filepath.Join(dir, "root-env.txt")
	subEnv := filepath.Join(dir, "sub-env.txt")

	// A hook that dumps the three variables it received into the given file.
	dumpEnv := func(name, out string) string {
		return writeHook(t, dir, name,
			`cat >/dev/null
{ echo "CLAUDECODE=$CLAUDECODE"; echo "CLAUDE_CODE_ENTRYPOINT=$CLAUDE_CODE_ENTRYPOINT"; echo "CLAUDE_CODE_SESSION_ID=$CLAUDE_CODE_SESSION_ID"; } > "`+out+`"`)
	}
	stopHook := dumpEnv("root-env.sh", rootEnv)
	startHook := dumpEnv("sub-env.sh", subEnv)
	writeSettings(t, dir, map[string]string{
		"Stop":          stopHook,
		"SubagentStart": startHook,
	})

	subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"subagent done","is_error":false}'
`)
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", subScript))

	// Run the mock with CLAUDECODE / CLAUDE_CODE_ENTRYPOINT explicitly BLANK in its
	// own process env (they are appended empty, overriding whatever the ambient dev
	// or CI shell carries). This proves the =1 / =sdk-cli the hooks observe is the value
	// the MOCK sets, not one forwarded from the surrounding environment.
	out, code := runInDir(t, dir, []string{"CLAUDECODE=", "CLAUDE_CODE_ENTRYPOINT="},
		"--script", orch, "--session-id", "sess-envtest", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// Root Stop hook saw all three, session id included.
	rootData, err := os.ReadFile(rootEnv)
	require.NoError(t, err, "root Stop hook must fire")
	root := string(rootData)
	assert.Contains(t, root, "CLAUDECODE=1", "root hook: CLAUDECODE")
	assert.Contains(t, root, "CLAUDE_CODE_ENTRYPOINT=sdk-cli", "root hook: CLAUDE_CODE_ENTRYPOINT")
	assert.Contains(t, root, "CLAUDE_CODE_SESSION_ID=sess-envtest", "root hook: CLAUDE_CODE_SESSION_ID")

	// Sub-agent SubagentStart hook saw the same harness env (session id shared).
	subData, err := os.ReadFile(subEnv)
	require.NoError(t, err, "SubagentStart hook must fire")
	sub := string(subData)
	assert.Contains(t, sub, "CLAUDECODE=1", "sub-agent hook: CLAUDECODE")
	assert.Contains(t, sub, "CLAUDE_CODE_ENTRYPOINT=sdk-cli", "sub-agent hook: CLAUDE_CODE_ENTRYPOINT")
	assert.Contains(t, sub, "CLAUDE_CODE_SESSION_ID=sess-envtest", "sub-agent hook: CLAUDE_CODE_SESSION_ID")
}
