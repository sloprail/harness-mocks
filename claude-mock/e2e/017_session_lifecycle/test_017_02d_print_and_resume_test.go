package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_16_NoStderrOutput: a Stop hook that fails without writing to stderr
// is recorded as "Failed with non-blocking status code: No stderr output", and
// its stop_hook_summary lists the error.
// staged:proves hook-exit-code-semantics/claude
// staged:proves hook-output-transcript-records/claude
func TestT017_16_NoStderrOutput(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	hook := write(t, filepath.Join(dir, "stop.sh"), "#!/bin/sh\ncat >/dev/null\nexit 1\n", 0o755)
	settings(t, dir, map[string]string{"Stop": hook})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "ne-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "ne-1"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Attachment["type"] == "hook_non_blocking_error" })
	require.GreaterOrEqual(t, at, 0)
	a := recs[at].Attachment
	assert.Equal(t, "Stop", a["hookName"])
	assert.Equal(t, "Failed with non-blocking status code: No stderr output", a["stderr"])
	assert.EqualValues(t, 1, a["exitCode"])
	assert.Equal(t, hook, a["command"])
	assert.Contains(t, a, "durationMs")
	require.True(t, isStopSummary(recs[at+1]))
	var s map[string]any
	require.NoError(t, json.Unmarshal([]byte(recs[at+1].Raw), &s))
	assert.Equal(t, []any{"Failed with non-blocking status code: No stderr output"}, s["hookErrors"])
	assert.Equal(t, true, s["hasOutput"])
}

// TestT017_17_UnknownResume: --resume of a session no transcript holds fails
// the way claude 2.1.282 fails it: "No conversation found with session ID:
// <id>" on stderr, an error result frame on stdout, exit 1, no SessionStart,
// SessionEnd fired, nothing written.
// staged:proves session-resume-unknown/claude
func TestT017_17_UnknownResume(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "SessionEnd": h})
	for _, extra := range [][]string{nil, {"--fork-session", "--session-id", "new-fork"}} {
		args := append([]string{"--script", script(t, dir, "s"), "--resume", "nosuch", "--project-dir", dir, "--config-dir", cfg}, extra...)
		out, code := runInDir(t, dir, nil, append(args, "-p", "hello")...)
		assert.Equal(t, 1, code, out)
		assert.Contains(t, out, "No conversation found with session ID: nosuch\n")
		assert.Contains(t, out, `"subtype":"error_during_execution"`)
		assert.Contains(t, out, `"errors":["No conversation found with session ID: nosuch"]`)
	}
	_, err := os.Stat(filepath.Join(cfg, "projects"))
	assert.True(t, os.IsNotExist(err), "nothing is written")
	var events []any
	for _, p := range payloads(t, log) {
		events = append(events, p["hook_event_name"])
	}
	assert.Equal(t, []any{"SessionEnd", "SessionEnd"}, events)
}

// TestT017_18_PrintMode: in raw --print mode Stop carries the output as
// last_assistant_message, and SessionEnd ends a `claude -p` session with
// reason "other" (claude 2.1.282).
// staged:proves session-end-hook/claude
// staged:proves stop-hook-payload/claude
func TestT017_18_PrintMode(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"Stop": h, "SessionEnd": h})
	sc := write(t, filepath.Join(dir, "s.sh"), "#!/bin/sh\necho 'RAW PRINT OUTPUT'\n", 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pm-1", "--print",
		"--project-dir", dir, "--config-dir", cfg, "hello")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "RAW PRINT OUTPUT")
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	assert.Equal(t, "Stop", ps[0]["hook_event_name"])
	assert.Equal(t, "RAW PRINT OUTPUT", ps[0]["last_assistant_message"])
	assert.Equal(t, false, ps[0]["stop_hook_active"])
	assert.Equal(t, []any{}, ps[0]["background_tasks"])
	assert.Equal(t, "SessionEnd", ps[1]["hook_event_name"])
	assert.Equal(t, "other", ps[1]["reason"])
	assert.NotContains(t, ps[1], "source")
}

// TestT017_22_ForkSessionWithoutResumeIsAPlainStart: --fork-session without
// --resume changes nothing — claude 2.1.282 started a plain session (source
// startup) under --session-id.
func TestT017_22_ForkSessionWithoutResumeIsAPlainStart(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--fork-session", "--session-id", "fs-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Equal(t, "startup", ps[0]["source"])
	root, _ := firstRoot(readRecs(t, transcriptPath(t, cfg, dir, "fs-1")))
	assert.Equal(t, "e2e-root-fs-1", root.UUID)
}

// TestT017_23_MainRecordsCarryRealBookkeeping: every record the session writes
// carries what every real one does — isSidechain false, userType, entrypoint
// "sdk-cli" (a `claude -p` run), version, gitBranch in a git repository.
// staged:proves transcript-record-envelope/claude
func TestT017_23_MainRecordsCarryRealBookkeeping(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	gitDir := filepath.Join(dir, "repo")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, exec.Command("git", "init", "-q", "-b", "feature-x", gitDir).Run())
	out, code := runInDir(t, gitDir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "bk-1",
		"--project-dir", gitDir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	for _, r := range readRecs(t, transcriptPath(t, cfg, gitDir, "bk-1")) {
		if r.UUID == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &m))
		assert.Equal(t, false, m["isSidechain"], r.Raw)
		assert.Equal(t, "external", m["userType"])
		assert.Equal(t, "sdk-cli", m["entrypoint"])
		assert.Equal(t, "2.1.282", m["version"])
		assert.Equal(t, "feature-x", m["gitBranch"])
		assert.Equal(t, "bk-1", m["sessionId"])
		assert.NotEmpty(t, m["timestamp"])
	}
}

// TestT017_24_EmptyToolResult: a tool that returns nothing is recorded as
// "(<Tool> completed with no output)" — claude 2.1.282 replaces empty result
// content with it; the real transcripts hold 3,479 such Bash results and no
// empty one. The structured result keeps the empty stdout.
// staged:proves empty-tool-result-placeholder/claude
func TestT017_24_EmptyToolResult(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "em-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	block, r := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "em-1")), "b1turn-s-a")
	assert.Equal(t, "(Bash completed with no output)", block["content"])
	assert.Equal(t, false, block["is_error"])
	assert.Equal(t, "", r.ToolUseResult["stdout"])
	assert.Contains(t, out, `"content":"(Bash completed with no output)"`, "the stream carries it too")
}
