package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With A10N_MOCK_NO_RESUME=1 a --resume that forks (--fork-session) is a resume of a
// session that does not exist like any other: the message and the error result name the
// session resumed, not the new id the fork would have had, and the run exits 1, the
// message on stderr and the error result on stdout (recorded for the unknown session: runs/resume-unknown).
// sr:proves no-resume
func TestT001_15_NoResumeNamesTheResumedSessionOfAFork(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho RAN\n"), 0o755))
	env := []string{"A10N_MOCK_NO_RESUME=1"}
	const resumed, fork = "00000000-0000-4000-8000-0000000000e1", "00000000-0000-4000-8000-0000000000e2"
	want := "No conversation found with session ID: " + resumed

	stdout, stderr, code := runSplit(t, dir, env, "--script", script, "--resume", resumed, "--fork-session", "--session-id", fork,
		"--project-dir", dir, "--output-format", "stream-json", "-p", "hello")
	assert.Equal(t, 1, code)
	assert.Equal(t, want, strings.TrimSpace(stderr))
	got := lastResult(t, stdout)
	assert.Equal(t, resumed, got["session_id"], "the error result names the session resumed")
	assert.Equal(t, true, got["is_error"])
	assert.NotContains(t, stdout, fork)
}
