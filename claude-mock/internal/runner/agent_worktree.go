package runner

import (
	"context"
	"path/filepath"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// hookedWorktree is the worktree a WorktreeCreate hook makes for an isolated
// sub-agent, when the project has one: the hook replaces Claude Code's own
// `git worktree` creation and prints the directory it made (recorded:
// snapshots/runs/worktree-hooks, where the hook is told the sub-agent's slug as
// `name`). hooked is false when no hook is configured, and the default creation
// goes ahead. A hook that fails, or prints no path, fails the creation.
//
// sr:provides worktree-hooks/claude
func hookedWorktree(ctx context.Context, cfg Config, inv *hooks.Invoker, name string) (path string, hooked bool, err error) {
	printed, hooked, err := subagents.WorktreeHook(true, func() (string, bool, error) {
		_, runs, e := inv.FireRuns(ctx, hooks.Input{
			SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventWorktreeCreate, WorktreeName: name,
		})
		if len(runs) == 0 && e == nil {
			return "", false, nil
		}
		stdout := ""
		if len(runs) > 0 {
			stdout = runs[0].Stdout
		}
		return stdout, true, e
	})
	if err != nil || !hooked {
		return "", hooked, err
	}
	if !filepath.IsAbs(printed) {
		printed = filepath.Join(cfg.Cwd, printed)
	}
	return filepath.Clean(printed), true, nil
}

// worktreeTrailer is the lines of the hand-back that name the worktree an
// isolated sub-agent ran in and its branch (recorded: snapshots/runs/isolated-worktree);
// a worktree a hook made has no branch (worktree-hooks).
func worktreeTrailer(path, branch string) string {
	if path == "" {
		return ""
	}
	if branch == "" {
		return "\nworktreePath: " + path
	}
	return "\nworktreePath: " + path + "\nworktreeBranch: " + branch
}
