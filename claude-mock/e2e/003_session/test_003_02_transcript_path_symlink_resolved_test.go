package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT003_02_TranscriptPathSymlinkResolved pins the fix for a real bug: the mock used to
// encode --project-dir DIRECTLY (no symlink resolution) into its transcript project-dir
// prefix, while real Claude Code resolves symlinks before encoding (verified empirically: a
// real claude run's own PreToolUse payload `cwd` field is ALREADY the resolved form, e.g.
// /private/tmp/... on macOS, not /tmp/...). This mismatch meant a consumer independently
// deriving "the transcript path for this cwd" via its OWN symlink-resolving logic (e.g.
// a10n-workspace's StableSessionID) could never find the file the mock ACTUALLY wrote to —
// the two encodings landed on different directory names for the exact same real directory.
//
// This drives the mock directly (no plugin/hooks needed) and asserts the session transcript
// file it writes lives under the SYMLINK-RESOLVED encoding of --project-dir.
// sr:proves session-transcript-file/claude
func TestT003_02_TranscriptPathSymlinkResolved(t *testing.T) {
	// The project directory is reached through a symlink of the test's own, on every
	// machine (a temp directory is a symlink only on some).
	realDir := t.TempDir()
	dir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realDir, dir))
	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	require.NotEqual(t, resolvedDir, dir)
	configDir := filepath.Join(realDir, "config")

	script := filepath.Join(dir, "root.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	const sessionID = "sess-symlink-1"
	out, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", sessionID, "--project-dir", dir, "--config-dir", configDir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// The encoded directory name must be derived from resolvedDir, not the raw (symlinked) dir.
	nonAlnum := regexp.MustCompile(`[^a-zA-Z0-9]`)
	wantEncoded := nonAlnum.ReplaceAllString(resolvedDir, "-")
	badEncoded := nonAlnum.ReplaceAllString(dir, "-")

	wantPath := filepath.Join(configDir, "projects", wantEncoded, sessionID+".jsonl")
	badPath := filepath.Join(configDir, "projects", badEncoded, sessionID+".jsonl")

	_, err = os.Stat(wantPath)
	assert.NoError(t, err, "transcript must exist at the RESOLVED-cwd-encoded path %q", wantPath)

	if badEncoded != wantEncoded {
		_, err = os.Stat(badPath)
		assert.Error(t, err, "transcript must NOT also exist at the unresolved-cwd-encoded path %q (the pre-fix location)", badPath)
	}
}
