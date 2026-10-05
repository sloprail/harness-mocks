package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_04_SubagentIdentity_RealClaude is an EMPIRICAL proof, against the REAL
// `claude` binary, of the two-axis subagent identity the mock (test_009_03) mirrors:
//
//   - session_id axis: a Task subagent SHARES the parent session_id (it is NOT the
//     agent_id and NOT a fresh session), in BOTH the SubagentStart/Stop hook payload
//     AND the subagent process's own CLAUDE_CODE_SESSION_ID env.
//   - cwd axis: with isolation="worktree" the subagent runs in a fresh worktree at
//     <parent>/.claude/worktrees/agent-<agentID>, so its cwd DIFFERS from the parent;
//     without isolation it shares the parent cwd. The two axes are INDEPENDENT.
//
// It is GUARDED so normal CI never runs it: it requires A10N_REAL_CLAUDE_SUBAGENT_TEST=1
// AND a `claude` binary on PATH. Spawning real claude costs tokens and may be absent, so
// without the gate the test t.Skips. This file is executable documentation: when claude's
// subagent semantics change, this is the test that catches it, and test_009_03 (the mock
// mirror) must be updated to match whatever this proves.
func TestT009_04_SubagentIdentity_RealClaude(t *testing.T) {
	if os.Getenv("A10N_REAL_CLAUDE_SUBAGENT_TEST") != "1" {
		t.Skip("set A10N_REAL_CLAUDE_SUBAGENT_TEST=1 to run the real-claude subagent-identity proof")
	}
	claudeBin, err := exec.LookPath("claude")
	require.NoError(t, err, "no `claude` binary on PATH: the real-claude proof was asked for (adr/tests-fail-on-missing-tool)")

	t.Run("worktree_isolation", func(t *testing.T) {
		p := runRealSubagent(t, claudeBin, true)
		// session_id axis: shared with parent, in payload AND subagent env.
		assert.Equal(t, p.rootSessionID, p.payloadSessionID,
			"SubagentStop payload session_id must equal the parent session_id")
		assert.Equal(t, p.rootSessionID, p.subEnvSessionID,
			"the subagent process CLAUDE_CODE_SESSION_ID must equal the parent session_id")
		assert.NotEqual(t, p.agentID, p.payloadSessionID,
			"session_id must NOT be the agent_id (they are distinct identifiers)")
		// cwd axis: isolated worktree, differs from parent, in payload AND subagent env.
		assert.NotEqual(t, p.rootCwd, p.payloadCwd,
			"worktree subagent payload cwd must differ from the parent cwd")
		assert.Contains(t, p.payloadCwd, filepath.Join(".claude", "worktrees", "agent-"),
			"worktree subagent cwd must be <parent>/.claude/worktrees/agent-<id>")
		assert.NotEqual(t, p.rootCwd, p.subEnvCwd,
			"the subagent process pwd must be its isolated worktree, not the parent")
	})

	t.Run("no_isolation", func(t *testing.T) {
		p := runRealSubagent(t, claudeBin, false)
		// session_id axis: still shared.
		assert.Equal(t, p.rootSessionID, p.subEnvSessionID,
			"without isolation the subagent still shares the parent session_id")
		// cwd axis: shares the parent cwd.
		assert.Equal(t, p.rootCwd, p.subEnvCwd,
			"without isolation the subagent shares the parent cwd")
	})
}

// realSubagentProbe holds the identity values observed from one real-claude run.
type realSubagentProbe struct {
	rootSessionID    string // parent CLAUDE_CODE_SESSION_ID (from root env)
	rootCwd          string // parent pwd
	subEnvSessionID  string // subagent process CLAUDE_CODE_SESSION_ID (from its env)
	subEnvCwd        string // subagent process pwd
	payloadSessionID string // SubagentStop hook payload session_id
	payloadCwd       string // SubagentStop hook payload cwd
	agentID          string // SubagentStop hook payload agent_id
}

// runRealSubagent drives a real `claude --print` that (a) records the root env, (b)
// spawns a Task subagent (optionally isolation="worktree") that records its own env,
// while a SubagentStop hook records the payload. Returns the parsed probe.
func runRealSubagent(t *testing.T, claudeBin string, worktree bool) realSubagentProbe {
	t.Helper()
	dir := t.TempDir()
	// A git repo is required for isolation="worktree" to bind a worktree.
	runGit(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")

	envLog := filepath.Join(dir, "env.log")
	payloadLog := filepath.Join(dir, "payload.log")

	// SubagentStop hook: append the raw payload JSON (one line) to payloadLog.
	hook := filepath.Join(dir, "stop.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >> \""+payloadLog+"\"\necho >> \""+payloadLog+"\"\n"), 0o755))
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settings := `{"hooks":{"SubagentStop":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644))

	isolation := ""
	if worktree {
		isolation = ` with isolation="worktree"`
	}
	prompt := `First run: ` + "`" + `echo "ROOT sid=$CLAUDE_CODE_SESSION_ID cwd=$(pwd)" >> ` + envLog + "`" + `. ` +
		`Then spawn a Task sub-agent (general-purpose)` + isolation + `. The sub-agent must run EXACTLY: ` +
		"`" + `echo "SUB sid=$CLAUDE_CODE_SESSION_ID cwd=$(pwd)" >> ` + envLog + "`" + `. Report done.`

	cmd := exec.Command(claudeBin, "--print", "--dangerously-skip-permissions", prompt)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("real claude run failed: %v\n%s", err, out)
	}

	var p realSubagentProbe
	// Parse env.log: two lines "ROOT|SUB sid=<> cwd=<>".
	envData, err := os.ReadFile(envLog)
	require.NoError(t, err, "env.log must exist (root + subagent both ran)")
	for _, line := range strings.Split(strings.TrimSpace(string(envData)), "\n") {
		sid := fieldAfter(line, "sid=")
		cwd := fieldAfter(line, "cwd=")
		switch {
		case strings.HasPrefix(line, "ROOT"):
			p.rootSessionID, p.rootCwd = sid, cwd
		case strings.HasPrefix(line, "SUB"):
			p.subEnvSessionID, p.subEnvCwd = sid, cwd
		}
	}
	require.NotEmpty(t, p.rootSessionID, "root env line missing: %s", envData)
	require.NotEmpty(t, p.subEnvSessionID, "subagent env line missing: %s", envData)

	// Parse the last SubagentStop payload.
	payloadData, err := os.ReadFile(payloadLog)
	require.NoError(t, err, "SubagentStop hook must have fired")
	for _, line := range strings.Split(strings.TrimSpace(string(payloadData)), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		if s, ok := m["session_id"].(string); ok && s != "" {
			p.payloadSessionID = s
		}
		if c, ok := m["cwd"].(string); ok && c != "" {
			p.payloadCwd = c
		}
		if a, ok := m["agent_id"].(string); ok && a != "" {
			p.agentID = a
		}
	}
	require.NotEmpty(t, p.payloadSessionID, "SubagentStop payload session_id missing")
	return p
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// fieldAfter returns the whitespace-delimited token immediately after key in s.
func fieldAfter(s, key string) string {
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
		return rest[:sp]
	}
	return rest
}
