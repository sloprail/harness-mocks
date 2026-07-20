package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_05_WorktreeSubagentActuallyRunsThere pins the fix for a real gap: prior to it,
// isolation="worktree" only made the mock REPORT a different cwd in hook payloads
// (SubagentStart/Stop JSON, the seeded transcript) — the subagent's actual Bash tool_use
// commands, and every hook command subprocess (including SubagentStop itself), still ran
// with the PARENT's real working directory regardless. A hook that shells out to a
// downstream tool resolving identity from os.Getwd() (e.g. a10n's `a10n-workspace session
// subagent-stop`, which reads session.Resolve()'s cwd, never the hook's JSON payload) would
// therefore always see the parent's tree, never the subagent's own isolated one — silently
// defeating the entire point of isolation for any consumer that (correctly) trusts its own
// process cwd over a claimed payload field.
//
// This proves BOTH halves now actually run in the bound worktree directory:
//  1. the subagent's own Bash tool_use `pwd` output.
//  2. the SubagentStop HOOK's own subprocess `pwd` (via its cmd.Dir, invoker.go) — the part
//     a downstream a10n-* binary's os.Getwd()-based identity resolution actually depends on.
//
// dir must be a REAL git repo with a commit for `git worktree add` to bind (bindWorktree,
// agent.go) — unlike the OTHER 009 tests, which use a plain (non-git) t.TempDir() and so
// exercise the plain-mkdir fallback path instead; this test exists specifically to prove the
// real `git worktree add` path.
func TestT009_05_WorktreeSubagentActuallyRunsThere(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")

	stopLog := filepath.Join(dir, "stop-pwd.log")
	// SubagentStop hook: record its OWN subprocess pwd — this is what the hook's cmd.Dir
	// actually resolved to, not what the JSON payload merely claims.
	stopHook := writeHook(t, dir, "stop.sh", `pwd >> "`+stopLog+`"`)
	writeSettings(t, dir, map[string]string{"SubagentStop": stopHook})

	subPwdLog := filepath.Join(dir, "sub-pwd.log")
	// The sub-agent SCRIPT ITSELF runs with cwd = subCwd (Config.Cwd passed to Run, see
	// runSubagent) — no Bash tool_use round-trip needed to observe it, matching the simpler
	// direct-side-effect pattern test_009_02 already uses.
	subScript := writeScript(t, dir, "sub.sh", `#!/bin/sh
pwd >> "`+subPwdLog+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
`)
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", subScript))

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "sess-wt-real", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// The bound worktree dir itself must exist as a real directory.
	worktreeGlob, err := filepath.Glob(filepath.Join(dir, ".claude", "worktrees", "agent-*"))
	require.NoError(t, err)
	require.Len(t, worktreeGlob, 1, "exactly one worktree dir must have been bound")
	worktreeDir := worktreeGlob[0]
	info, err := os.Stat(worktreeDir)
	require.NoError(t, err, "the worktree directory must actually exist on disk")
	assert.True(t, info.IsDir())
	// Symlink-resolve for comparison against `pwd` output below: on macOS /var is a symlink
	// to /private/var, so t.TempDir()'s own path string and a subprocess's `pwd` (which
	// resolves symlinks) differ in spelling for the SAME real directory.
	wantPwd, err := filepath.EvalSymlinks(worktreeDir)
	require.NoError(t, err)

	// It is a REAL git worktree (not just an empty directory) — `git rev-parse --is-inside-work-tree`
	// succeeds inside it, and it shares the same repo as the parent.
	isWT := exec.Command("git", "-C", worktreeDir, "rev-parse", "--is-inside-work-tree")
	wtOut, wtErr := isWT.CombinedOutput()
	require.NoError(t, wtErr, "worktree dir must be a real git worktree: %s", wtOut)
	assert.Equal(t, "true", strings.TrimSpace(string(wtOut)))

	// The subagent's OWN Bash tool_use actually ran INSIDE the worktree.
	subPwd, err := os.ReadFile(subPwdLog)
	require.NoError(t, err, "subagent's pwd command must have run")
	assert.Equal(t, wantPwd, strings.TrimSpace(string(subPwd)),
		"the subagent's own Bash tool_use must run in the bound worktree, not the parent dir")

	// The SubagentStop HOOK SUBPROCESS itself actually ran INSIDE the worktree — this is the
	// part any os.Getwd()-based downstream tool (e.g. a10n-workspace) depends on.
	stopPwd, err := os.ReadFile(stopLog)
	require.NoError(t, err, "SubagentStop hook must have fired")
	assert.Equal(t, wantPwd, strings.TrimSpace(string(stopPwd)),
		"the SubagentStop hook's OWN subprocess must run in the bound worktree, not the parent dir")
}
