package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_16_CleanWorktreeIsRemovedAndTheSidecarSaysSo: an isolated sub-agent that
// leaves its worktree as it found it has the worktree and its branch removed when it
// finishes, and its sidecar loses worktreePath, spawnedWithWorktree and worktreeBranch
// and gains worktreeCleanlyRemoved: true (recorded: snapshots/runs/meta, the "iso"
// sub-agent). One that left a file, edited a tracked file or committed (on its branch or a new one) keeps its worktree, its branch and the fields.
// sr:proves subagent-worktree-isolation/claude
func TestT009_16_CleanWorktreeIsRemovedAndTheSidecarSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		kept       bool
	}{
		{"clean", "true", false},
		{"left-a-file", "touch left-behind.txt", true},
		{"edited-tracked-file", "echo changed > f", true},
		{"committed", "git -c user.email=t@t -c user.name=t commit -q --allow-empty -m work", true},
		{"committed-on-new-branch", "git switch -q -c other && git -c user.email=t@t -c user.name=t commit -q --allow-empty -m work", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			runGit(t, dir, "init", "-q")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644))
			runGit(t, dir, "add", "-A")
			runGit(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")
			sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\n"+tc.body+"\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"sub done\",\"is_error\":false}'\n")
			orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", sub))
			cfg := filepath.Join(dir, "config")
			out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "wt-clean", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, out)

			worktrees, err := filepath.Glob(filepath.Join(dir, ".claude", "worktrees", "agent-*"))
			require.NoError(t, err)
			branches, err := exec.Command("git", "-C", dir, "branch", "--list", "worktree-agent-*").Output()
			require.NoError(t, err)
			sidecars, err := filepath.Glob(filepath.Join(cfg, "projects", "*", "wt-clean", "subagents", "*.meta.json"))
			require.NoError(t, err)
			require.Len(t, sidecars, 1)
			raw, err := os.ReadFile(sidecars[0])
			require.NoError(t, err)
			var meta map[string]any
			require.NoError(t, json.Unmarshal(raw, &meta))
			if tc.kept {
				assert.Len(t, worktrees, 1, "a worktree with work stays")
				assert.NotEmpty(t, branches)
				assert.Equal(t, true, meta["spawnedWithWorktree"])
				assert.NotEmpty(t, meta["worktreePath"])
				assert.NotEmpty(t, meta["worktreeBranch"])
				assert.NotContains(t, meta, "worktreeCleanlyRemoved")
				assert.Contains(t, out, "worktreePath: ", "the hand-back names a worktree that is kept")
				return
			}
			assert.Empty(t, worktrees, "a clean worktree is removed")
			assert.Empty(t, branches, "and so is its branch")
			assert.Equal(t, true, meta["worktreeCleanlyRemoved"])
			for _, gone := range []string{"worktreePath", "spawnedWithWorktree", "worktreeBranch"} {
				assert.NotContains(t, meta, gone)
			}
			assert.NotContains(t, out, "worktreePath", "the hand-back of a removed worktree names none (recorded: the meta run's iso sub-agent)")
		})
	}
}
