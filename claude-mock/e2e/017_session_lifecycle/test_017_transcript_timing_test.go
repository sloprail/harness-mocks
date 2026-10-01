package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_32_TranscriptExistsFromTheFirstRecordOn: a fresh session's transcript file
// does not exist when SessionStart runs, nor when UserPromptSubmit runs (the prompt is
// written once that hook has let it through), and does at Stop and SessionEnd
// (recorded: snapshots/runs/transcript-at-start).
// sr:proves session-transcript-file/claude
func TestT017_32_TranscriptExistsFromTheFirstRecordOn(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "exists.log")
	hook := write(t, filepath.Join(dir, "probe.sh"), `#!/bin/sh
IN=$(cat)
EV=$(printf '%s' "$IN" | sed -n 's/.*"hook_event_name":"\([A-Za-z]*\)".*/\1/p')
P=$(printf '%s' "$IN" | sed -n 's/.*"transcript_path":"\([^"]*\)".*/\1/p')
if [ -f "$P" ]; then echo "$EV exists" >> `+log+`; else echo "$EV missing" >> `+log+`; fi
`, 0o755)
	settings(t, dir, map[string]string{"SessionStart": hook, "UserPromptSubmit": hook, "Stop": hook, "SessionEnd": hook})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "tt-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, []string{"SessionStart missing", "UserPromptSubmit missing", "Stop exists", "SessionEnd exists"},
		strings.Split(strings.TrimSpace(string(data)), "\n"))
}

// TestT017_33_AResumedSessionsTranscriptExistsAtItsStart: resuming a session, its
// transcript is on disk already when SessionStart runs.
// sr:proves session-transcript-file/claude
func TestT017_33_AResumedSessionsTranscriptExistsAtItsStart(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "exists.log")
	hook := write(t, filepath.Join(dir, "probe.sh"), `#!/bin/sh
IN=$(cat)
P=$(printf '%s' "$IN" | sed -n 's/.*"transcript_path":"\([^"]*\)".*/\1/p')
if [ -f "$P" ]; then echo exists >> `+log+`; else echo missing >> `+log+`; fi
`, 0o755)
	settings(t, dir, map[string]string{"SessionStart": hook})
	for _, args := range [][]string{{"--session-id", "tt-2"}, {"--resume", "tt-2"}} {
		out, code := runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "s"), "--project-dir", dir, "--config-dir", cfg, "-p", "go"}, args...)...)
		require.Equal(t, 0, code, out)
	}
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, []string{"missing", "exists"}, strings.Split(strings.TrimSpace(string(data)), "\n"))
}
