package session

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
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

// gitBranch is the branch checked out in the repository containing dir, or ""
// outside a repository or on a detached HEAD.
func gitBranch(dir string) string {
	res, err := procexec.Run(context.Background(), procexec.Spec{
		Argv: []string{"git", "symbolic-ref", "--short", "-q", "HEAD"}, Dir: dir,
		Env: os.Environ(), Timeout: 5 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(res.Stdout))
}
