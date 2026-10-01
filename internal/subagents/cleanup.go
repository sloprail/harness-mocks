package subagents

import (
	"context"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// CleanupWorktree removes the worktree and the branch of a sub-agent that
// finished, when it left them as it found them: nothing uncommitted in the
// worktree and no commit on its branch. A worktree that holds work stays. It
// reports whether it removed it.
func CleanupWorktree(ctx context.Context, parentCwd string, wt Worktree) bool {
	run := func(dir string, args ...string) (string, bool) {
		res, err := procexec.Run(ctx, procexec.Spec{Argv: append([]string{"git", "-C", dir}, args...)})
		return strings.TrimSpace(string(res.Stdout)), err == nil && res.ExitCode == 0
	}
	if out, ok := run(wt.Path, "status", "--porcelain"); !ok || out != "" {
		return false
	}
	if out, ok := run(parentCwd, "rev-list", "--count", "HEAD.."+wt.Branch); !ok || out != "0" {
		return false
	}
	if _, ok := run(parentCwd, "worktree", "remove", "--force", wt.Path); !ok {
		return false
	}
	run(parentCwd, "branch", "-D", wt.Branch)
	return true
}
