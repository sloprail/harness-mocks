package subagents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// WorktreeLayout is where a harness puts a sub-agent's isolated worktree and
// what it names the branch: <parent>/<Dir>/<Prefix><id> on <BranchPrefix><id>.
type WorktreeLayout struct {
	Dir, Prefix, BranchPrefix string
}

// Worktree is an isolated working copy a sub-agent runs in.
type Worktree struct {
	Path, Branch string
}

// Isolation is where a sub-agent dispatched with isolation runs.
type Isolation struct {
	// Cwd is its working directory: where its tools and hooks run. The session id
	// stays the parent's.
	Cwd string
	// Worktree is its worktree and new branch; nil when it could not be a real
	// one (a plain directory, or the parent's own).
	Worktree *Worktree
	// Notes are what the harness reports of a fallback.
	Notes []string
}

// Isolate puts sub-agent id in its own worktree on a new branch, under
// parentCwd. bind makes the directory a real worktree; when it cannot, the
// sub-agent gets a plain directory, and when that cannot be made either it
// shares the parent's.
func Isolate(parentCwd, id string, l WorktreeLayout, bind func(dir, branch string) error) Isolation {
	dir := filepath.Join(parentCwd, l.Dir, l.Prefix+id)
	branch := l.BranchPrefix + id
	if err := bind(dir, branch); err == nil {
		return Isolation{Cwd: dir, Worktree: &Worktree{Path: dir, Branch: branch}}
	} else if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
		return Isolation{Cwd: dir, Notes: []string{fmt.Sprintf("bind %s: %v (falling back to a plain directory)", dir, err)}}
	} else {
		return Isolation{Cwd: parentCwd, Notes: []string{fmt.Sprintf("mkdir %s: %v: isolation not applied, sharing parent cwd", dir, mkErr)}}
	}
}

// BindGit makes dir a real git worktree of parentCwd's HEAD on a new branch. It
// fails when parentCwd is not a git repository with a commit.
func BindGit(ctx context.Context, parentCwd string) func(dir, branch string) error {
	git := func(args ...string) (procexec.Result, error) {
		res, err := procexec.Run(ctx, procexec.Spec{Argv: append([]string{"git", "-C", parentCwd}, args...)})
		if err == nil && res.ExitCode != 0 {
			err = fmt.Errorf("git %s: exit %d: %s", args[0], res.ExitCode, strings.TrimSpace(string(res.Stderr)))
		}
		return res, err
	}
	return func(dir, branch string) error {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return fmt.Errorf("mkdir parent: %w", err)
		}
		if _, err := git("rev-parse", "--is-inside-work-tree"); err != nil {
			return fmt.Errorf("not a git repo: %w", err)
		}
		if _, err := git("rev-parse", "--verify", "HEAD"); err != nil {
			return fmt.Errorf("no HEAD (no commits yet): %w", err)
		}
		if _, err := git("worktree", "add", "-b", branch, dir, "HEAD"); err != nil {
			return fmt.Errorf("git worktree add: %w", err)
		}
		return nil
	}
}

// WorktreeHook fires the hook for the harness creating or removing an isolated
// worktree, and returns the hook's failure: a failing create hook aborts the
// creation.
func WorktreeHook(fire func() error) error { return fire() }
