package runner

import (
	"context"
	"fmt"
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
	var runs []hooks.HandlerRun
	err = subagents.WorktreeHook(func() error {
		_, r, e := inv.FireRuns(ctx, hooks.Input{
			SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventWorktreeCreate, WorktreeName: name,
		})
		runs = r
		return e
	})
	if len(runs) == 0 && err == nil {
		return "", false, nil
	}
	if err != nil {
		return "", true, err
	}
	printed, ok := subagents.HookWorktreePath(runs[0].Stdout)
	if !ok {
		return "", true, fmt.Errorf("the WorktreeCreate hook printed no worktree path")
	}
	if !filepath.IsAbs(printed) {
		printed = filepath.Join(cfg.Cwd, printed)
	}
	return filepath.Clean(printed), true, nil
}
