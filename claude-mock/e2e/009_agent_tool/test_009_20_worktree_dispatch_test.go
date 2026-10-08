package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dispatchRepo is a git repo whose origin/HEAD is its first commit and whose local HEAD is a
// second one, so the commit a sub-agent's worktree starts at tells which base it took.
func dispatchRepo(t *testing.T) (dir, first, second string) {
	t.Helper()
	dir = t.TempDir()
	commit := func(msg string) string {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte(msg), 0o644))
		runGit(t, dir, "add", "-A")
		runGit(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", msg)
		return gitOut(t, dir, "rev-parse", "HEAD")
	}
	runGit(t, dir, "init", "-q")
	first = commit("first")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", first)
	runGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	second = commit("second")
	return dir, first, second
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

// dispatchScript is an orchestrator that calls Agent once with the extra input fields given.
func dispatchScript(subScript, extra string) string {
	return fmt.Sprintf(`#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
printf '%%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_1","name":"Agent","input":{"description":"d","prompt":"go",%s,"script":"%s"}}]}}'
`, extra, subScript)
}

// runIsolated dispatches one sub-agent (which leaves a file, so its worktree stays) with the
// Agent input fields given and returns the one worktree left.
func runIsolated(t *testing.T, dir, agentInput string) string {
	t.Helper()
	sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\ntouch left-behind.txt\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"sub done\",\"is_error\":false}'\n")
	orch := writeScript(t, dir, "orch.sh", dispatchScript(sub, agentInput))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "wt-dispatch", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "go")
	require.Equal(t, 0, code, out)
	worktrees, err := filepath.Glob(filepath.Join(dir, ".claude", "worktrees", "agent-*"))
	require.NoError(t, err)
	require.Len(t, worktrees, 1, out)
	return worktrees[0]
}

// A custom sub-agent whose frontmatter sets `isolation: worktree` always runs in its own
// worktree, though the Agent call asks for no isolation (the worktrees doc, "Isolate
// subagents with worktrees"; no run under snapshots/runs records a definition with it,
// snapshots/runs/isolated-worktree records the call's own isolation).
// sr:proves subagent-worktree-isolation/claude
func TestT009_20_DefinitionIsolationWorktreeAlwaysIsolates(t *testing.T) {
	dir, _, _ := dispatchRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "agents", "refactorer.md"),
		[]byte("---\nname: refactorer\ndescription: refactors\nisolation: worktree\n---\n\nRefactor.\n"), 0o644))
	wt := runIsolated(t, dir, `"subagent_type":"refactorer"`)
	assert.FileExists(t, filepath.Join(wt, "left-behind.txt"), "the sub-agent's tools ran in the worktree")
}

// A sub-agent's worktree branches from the repository's default branch (origin/HEAD, the
// "fresh" base) unless `worktree.baseRef` is "head", which branches from the current HEAD.
// sr:proves subagent-worktree-isolation/claude
func TestT009_20_WorktreeBaseIsTheDefaultBranchUnlessBaseRefIsHead(t *testing.T) {
	for _, tc := range []struct {
		name, settings string
		fromFirst      bool
	}{
		{"fresh-by-default", "", true},
		{"fresh", `{"worktree":{"baseRef":"fresh"}}`, true},
		{"head", `{"worktree":{"baseRef":"head"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, first, second := dispatchRepo(t)
			if tc.settings != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(tc.settings), 0o644))
			}
			wt := runIsolated(t, dir, `"isolation":"worktree"`)
			want := second
			if tc.fromFirst {
				want = first
			}
			assert.Equal(t, want, gitOut(t, wt, "rev-parse", "HEAD"))
		})
	}
}
