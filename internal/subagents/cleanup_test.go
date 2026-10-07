package subagents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanupWorktree_RemovesACleanOneAndKeepsOneWithWork(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	bind := BindGit(context.Background(), repo)

	clean := Isolate(repo, "clean", wl(), bind)
	if !clean.Cleanup(context.Background()) {
		t.Fatal("a worktree left as it was found is not removed")
	}
	if _, err := os.Stat(clean.Cwd); !os.IsNotExist(err) {
		t.Fatal("the worktree directory is still there")
	}
	if out, _ := exec.Command("git", "-C", repo, "branch", "--list", "worktree-agent-clean").Output(); len(out) != 0 {
		t.Fatal("the branch is still there")
	}

	dirty := Isolate(repo, "dirty", wl(), bind)
	if err := os.WriteFile(filepath.Join(dirty.Cwd, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirty.Cleanup(context.Background()) {
		t.Fatal("a worktree with an uncommitted file was removed")
	}

	committed := Isolate(repo, "committed", wl(), bind)
	git(t, committed.Cwd, "commit", "-q", "--allow-empty", "-m", "work")
	if committed.Cleanup(context.Background()) {
		t.Fatal("a worktree whose branch has a commit was removed")
	}
	if _, err := os.Stat(committed.Cwd); err != nil {
		t.Fatalf("the kept worktree is gone: %v", err)
	}
}

// A sub-agent that switches to another branch and commits there left work the
// agent branch does not show: the worktree is kept. One that only switched
// branch, changing nothing, is removed.
func TestCleanupWorktree_KeepsOneWhoseHeadMovedToACommittedBranch(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	bind := BindGit(context.Background(), repo)

	switched := Isolate(repo, "switched", wl(), bind)
	git(t, switched.Cwd, "switch", "-q", "-c", "other")
	git(t, switched.Cwd, "commit", "-q", "--allow-empty", "-m", "work")
	if switched.Cleanup(context.Background()) {
		t.Fatal("a worktree that committed on a new branch was removed")
	}
	if _, err := os.Stat(switched.Cwd); err != nil {
		t.Fatalf("the kept worktree is gone: %v", err)
	}

	moved := Isolate(repo, "moved", wl(), bind)
	git(t, moved.Cwd, "switch", "-q", "-c", "empty")
	if !moved.Cleanup(context.Background()) {
		t.Fatal("a worktree on another branch with no changes is kept")
	}
}
