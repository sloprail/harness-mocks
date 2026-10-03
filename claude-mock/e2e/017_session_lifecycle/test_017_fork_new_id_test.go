package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_77_ForkWithoutAnIdGetsItsOwnAsRecorded: `--resume <id> --fork-session`
// with no --session-id continues under a NEW session id, the one the fork's
// hooks and the stream's init frame report (the SDK docs' forkedId), in a new
// transcript that carries the original's history; the original is untouched. The
// forkresume run's fork carries the original turn (echo ORIGINAL) under its own
// session id, as the mock's does.
// sr:proves session-fork/claude
func TestT017_77_ForkWithoutAnIdGetsItsOwnAsRecorded(t *testing.T) {
	const orig = "00000000-0000-4000-8000-0000000000a1"
	realFork := recordedFile(t, "../../snapshots/runs/forkresume/samples/*/transcript/00000000-0000-4000-8000-0000000000b2.jsonl")
	realRaw, err := os.ReadFile(realFork)
	require.NoError(t, err)
	assert.Contains(t, string(realRaw), "echo ORIGINAL", "recorded: the fork carries the original's turn")
	var realStreamID string
	rawStream, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/forkresume/samples/*/stream.jsonl"))
	require.NoError(t, err)
	for _, l := range strings.Split(string(rawStream), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["subtype"] == "hook_started" && f["hook_name"] == "SessionStart:fork" {
			realStreamID, _ = f["session_id"].(string)
		}
	}
	assert.Equal(t, "00000000-0000-4000-8000-0000000000b2", realStreamID, "recorded: the stream's fork frames name the fork's id")
	for _, r := range readRecs(t, realFork) {
		if r.SessionID != "" {
			assert.Equal(t, "00000000-0000-4000-8000-0000000000b2", r.SessionID, "recorded: every record under the fork's id")
		}
	}

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a", toolUse("b1", "Bash", `{"command":"echo ORIGINAL"}`)), "--session-id", orig,
		"--project-dir", dir, "--config-dir", cfg, "-p", "first")
	require.Equal(t, 0, code, out)
	origPath := transcriptPath(t, cfg, dir, orig)
	before, err := os.ReadFile(origPath)
	require.NoError(t, err)

	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "b"), "--resume", orig, "--fork-session",
		"--project-dir", dir, "--config-dir", cfg, "-p", "second")
	require.Equal(t, 0, code, out)

	var initID string
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["subtype"] == "hook_started" && f["hook_name"] == "SessionStart:fork" {
			initID, _ = f["session_id"].(string)
		}
	}
	require.NotEmpty(t, initID, "the stream's SessionStart:fork frame reports the session")
	assert.NotEqual(t, orig, initID, "a new id, not the original's")
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	assert.Equal(t, "fork", ps[1]["source"])
	assert.Equal(t, initID, ps[1]["session_id"], "the hooks and the stream name the same fork")

	forkPath := transcriptPath(t, cfg, dir, initID)
	forkRaw, err := os.ReadFile(forkPath)
	require.NoError(t, err)
	assert.Contains(t, string(forkRaw), "echo ORIGINAL", "the fork carries the original's history")
	assert.Contains(t, string(forkRaw), "second")
	for _, r := range readRecs(t, forkPath) {
		if r.SessionID != "" {
			assert.Equal(t, initID, r.SessionID)
		}
	}
	after, err := os.ReadFile(origPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the original is untouched")
}
