package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentCallThen is an orchestrator that dispatches one isolated sub-agent running
// subScript, and ends once the session file holds done.
func agentCallThen(subScript, done string) string {
	return `#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q '` + done + `' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_1","name":"Agent","input":{"description":"iso","prompt":"go","subagent_type":"general-purpose","isolation":"worktree","script":"` + subScript + `"}}]}}'
`
}

// TestT009_17_WorktreeCreateHookReplacesTheIsolationAndReturnsThePath: for an
// isolated sub-agent, a WorktreeCreate hook is told the sub-agent's slug as
// `name` and replaces Claude Code's own worktree creation: the directory it prints
// (the last non-empty stdout line, relative to the session's directory) is where
// the sub-agent runs (recorded: snapshots/runs/worktree-hooks; hooks#worktreecreate).
// sr:proves worktree-hooks/claude
func TestT009_17_WorktreeCreateHookReplacesTheIsolationAndReturnsThePath(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "create.log")
	hook := writeHook(t, dir, "create.sh", `IN=$(cat); printf '%s\n' "$IN" >> `+log+`
NAME=$(printf '%s' "$IN" | sed -n 's/.*"name":"\([^"]*\)".*/\1/p')
mkdir -p "made/$NAME"; echo "banner on stdout"; echo "made/$NAME"`)
	writeSettings(t, dir, map[string]string{"WorktreeCreate": hook})
	pwdLog := filepath.Join(dir, "sub-pwd.log")
	sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\npwd >> "+pwdLog+"\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"sub done\",\"is_error\":false}'\n")
	out, code := runInDir(t, dir, nil, "--script", writeScript(t, dir, "orch.sh", orchestratorScript("Agent", sub)),
		"--session-id", "wt-hook", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, out)

	payload, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"hook_event_name":"WorktreeCreate"`)
	assert.Regexp(t, `"name":"agent-a[0-9a-f]{16}"`, string(payload), "the slug of the sub-agent")
	made, err := filepath.Glob(filepath.Join(dir, "made", "agent-a*"))
	require.NoError(t, err)
	require.Len(t, made, 1)
	want, err := filepath.EvalSymlinks(made[0])
	require.NoError(t, err)
	got, err := os.ReadFile(pwdLog)
	require.NoError(t, err)
	assert.Equal(t, want, strings.TrimSpace(string(got)), "the sub-agent runs where the hook said")
	worktrees, _ := filepath.Glob(filepath.Join(dir, ".claude", "worktrees", "agent-*"))
	assert.Empty(t, worktrees, "the hook replaced the default creation")
}

// TestT009_17b_WorktreeCreateHookThatFailsOrPrintsNoPathFailsTheCreation: "If the
// hook fails or produces no path, worktree creation fails with an error"
// (hooks#worktreecreate-output): the sub-agent does not run.
// sr:proves worktree-hooks/claude
func TestT009_17b_WorktreeCreateHookThatFailsOrPrintsNoPathFailsTheCreation(t *testing.T) {
	for name, body := range map[string]string{"exit-1": "cat >/dev/null; echo oops >&2; exit 1", "no-path": "cat >/dev/null"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			ran := filepath.Join(dir, "sub-ran")
			hook := writeHook(t, dir, "create.sh", body)
			writeSettings(t, dir, map[string]string{"WorktreeCreate": hook})
			sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\ntouch "+ran+"\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"sub done\",\"is_error\":false}'\n")
			out, code := runInDir(t, dir, nil, "--script", writeScript(t, dir, "orch.sh", agentCallThen(sub, "could not create the worktree")),
				"--session-id", "wt-hook-fail", "--project-dir", dir, "-p", "go")
			require.Equal(t, 0, code, out)
			assert.Contains(t, out, "could not create the worktree")
			_, err := os.Stat(ran)
			assert.True(t, os.IsNotExist(err), "the sub-agent never ran")
		})
	}
}
