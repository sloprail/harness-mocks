package session

import (
	"os/exec"
	"strings"
	"time"
)

// meta is the payload of a rollout's first record, the one place codex writes
// the session's bookkeeping: the session id, when it started, the working
// directory, how the run was launched (originator and source "exec" for a
// non-interactive run), the version and the git branch. Later records carry
// only their timestamp (recorded: runs/session-transcript-file).
//
// sr:provides transcript-record-envelope/codex
func meta(id, cwd string, now time.Time) map[string]any {
	m := map[string]any{"id": id, "session_id": id, "timestamp": now.UTC().Format(time.RFC3339Nano),
		"cwd": cwd, "originator": "codex_exec", "source": "exec", "cli_version": "mock"}
	if b := gitBranch(cwd); b != "" {
		m["git"] = map[string]any{"branch": b}
	}
	return m
}

// gitBranch is the branch checked out in dir, or "" outside a repository.
func gitBranch(dir string) string {
	out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
