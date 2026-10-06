package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A10N_MOCK_NO_RESUME=1 makes every --resume behave as one naming an unknown
// session, whatever shape the resume has (spec/invariants/no-resume).
func noResumeRun(t *testing.T, env []string, args ...string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	assert.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"x\"}'\n"), 0o755))
	base := []string{"--script", script, "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go"}
	return runInDir(t, dir, env, append(args, base...)...)
}

// sr:proves session-resume-unknown/claude
// sr:invariant no-resume
func TestNoResumeRefusesAResumeWithAFork(t *testing.T) {
	out, code := noResumeRun(t, []string{"A10N_MOCK_NO_RESUME=1"}, "--resume", "s-1", "--fork-session", "--output-format", "stream-json")
	assert.Equal(t, 1, code, out)
	assert.Contains(t, out, "No conversation found with session ID: ") // names the id the fork runs under, a fresh one
}

// A session that was never started is unknown without the env var as well, and
// the refusal is the same.
// sr:proves session-resume-unknown/claude
func TestAnUnknownSessionIsRefusedWithoutTheEnvVar(t *testing.T) {
	out, code := noResumeRun(t, nil, "--resume", "s-never", "--output-format", "stream-json")
	assert.Equal(t, 1, code, out)
	assert.Contains(t, out, "No conversation found with session ID: s-never")
}
