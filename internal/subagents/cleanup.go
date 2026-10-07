package subagents

import (
	"context"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// cleanupWorktree removes the worktree and the branch of a sub-agent that
// finished, when it left them as it found them: nothing uncommitted in the
// worktree (untracked files count) and no commit beyond the commit it started at, on
// whatever branch the worktree's HEAD is on. A worktree
// that holds work stays. It reports whether it removed it.
func cleanupWorktree(ctx context.Context, parentCwd string, wt Worktree) bool {
	run := func(dir string, args ...string) (string, bool) {
		res, err := procexec.Run(ctx, procexec.Spec{Argv: append([]string{"git", "-C", dir}, args...)})
		return strings.TrimSpace(string(res.Stdout)), err == nil && res.ExitCode == 0
	}
	if out, ok := run(wt.Path, "status", "--porcelain"); !ok || out != "" {
		return false
	}
	if wt.Start == "" {
		return false
	}
	if out, ok := run(wt.Path, "rev-list", "--count", wt.Start+"..HEAD"); !ok || out != "0" {
		return false
	}
	if _, ok := run(parentCwd, "worktree", "remove", "--force", wt.Path); !ok {
		return false
	}
	run(parentCwd, "branch", "-D", wt.Branch)
	return true
}
