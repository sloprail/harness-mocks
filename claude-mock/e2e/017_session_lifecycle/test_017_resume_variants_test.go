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

// TestT017_80_ContinueSkipsOtherDirectories: --continue loads the most recent
// session of the CURRENT directory (the CLI reference); a newer session of
// another directory is not taken, and a directory with none is an error.
// sr:proves session-resume/claude
func TestT017_80_ContinueSkipsOtherDirectories(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "config")
	here, there := filepath.Join(root, "here"), filepath.Join(root, "there")
	require.NoError(t, os.MkdirAll(here, 0o755))
	require.NoError(t, os.MkdirAll(there, 0o755))
	for _, tc := range []struct{ dir, id string }{{here, "mine"}, {there, "theirs"}} {
		out, code := runInDir(t, tc.dir, nil, "--script", script(t, tc.dir, tc.id), "--session-id", tc.id,
			"--project-dir", tc.dir, "--config-dir", cfg, "-p", "first")
		require.Equal(t, 0, code, out)
	}
	out, code := runInDir(t, here, nil, "--script", script(t, here, "c"), "--continue",
		"--project-dir", here, "--config-dir", cfg, "-p", "go on")
	require.Equal(t, 0, code, out)
	assert.Contains(t, readString(t, transcriptPath(t, cfg, here, "mine")), "go on", "this directory's session, though the other is newer")
	assert.NotContains(t, readString(t, transcriptPath(t, cfg, there, "theirs")), "go on")
}

// TestT017_81_ContinueThenForkAsRecorded: `--continue --fork-session` forks the
// directory's most recent session instead of appending to it, as the
// resume-continue-fork run did: SessionStart source "fork" under the new id, a
// new transcript that carries the original's turn, and the original untouched.
// sr:proves session-fork/claude
func TestT017_81_ContinueThenForkAsRecorded(t *testing.T) {
	const orig, fork = "00000000-0000-4000-8000-0000000000d1", "00000000-0000-4000-8000-0000000000d2"
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/resume-continue-fork/samples/*/payloads.jsonl"))
	require.NoError(t, err)
	for _, l := range strings.Split(string(raw), "\n") {
		var p map[string]any
		if json.Unmarshal([]byte(l), &p) == nil && p["hook_event_name"] == "SessionStart" {
			assert.Equal(t, "fork", p["source"])
			assert.Equal(t, fork, p["session_id"], "recorded: the fork's own id")
		}
	}
	realFork := readString(t, recordedFile(t, "../../snapshots/runs/resume-continue-fork/samples/*/transcript/"+fork+".jsonl"))
	assert.Contains(t, realFork, "Reply only ONE", "recorded: the fork carries the original's turn")
	assert.Contains(t, realFork, "Reply only TWO")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a"), "--session-id", orig,
		"--project-dir", dir, "--config-dir", cfg, "-p", "Reply only ONE")
	require.Equal(t, 0, code, out)
	before := readString(t, transcriptPath(t, cfg, dir, orig))
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "b"), "--continue", "--fork-session", "--session-id", fork,
		"--project-dir", dir, "--config-dir", cfg, "-p", "Reply only TWO")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	assert.Equal(t, "fork", ps[1]["source"])
	assert.Equal(t, fork, ps[1]["session_id"])
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["type"] == "system" && f["session_id"] != nil {
			assert.Equal(t, fork, f["session_id"], "every system frame the mock writes names the fork: %s", l)
		}
	}
	forked := readString(t, transcriptPath(t, cfg, dir, fork))
	assert.Contains(t, forked, "Reply only ONE")
	assert.Contains(t, forked, "Reply only TWO")
	assert.Equal(t, before, readString(t, transcriptPath(t, cfg, dir, orig)), "the original is untouched")
}

// TestT017_82_NoSessionPersistenceLeavesNothingToResume: with
// --no-session-persistence a run leaves no transcript, as the recorded run left
// none (its transcript folder is empty), so the session cannot be resumed.
// sr:proves session-resume/claude
func TestT017_82_NoSessionPersistenceLeavesNothingToResume(t *testing.T) {
	recorded, err := filepath.Glob("../../snapshots/runs/no-session-persistence/samples/*/transcript/*.jsonl")
	require.NoError(t, err)
	assert.Empty(t, recorded, "recorded: no transcript was written")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "np-1", "--no-session-persistence",
		"--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, out)
	assert.NoFileExists(t, transcriptPath(t, cfg, dir, "np-1"))
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "b"), "--resume", "np-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "again")
	assert.NotEqual(t, 0, code, out)
	assert.Contains(t, out, "No conversation found with session ID: np-1")
}

// TestT017_83_ResumeFromAnotherDirectoryAsRecorded: a session resumed from a
// subdirectory is told that directory as its cwd by every hook, while the
// SessionStart's transcript_path is where the NEW directory's project folder would
// hold it (no such file) and the hooks after it are told the original's file —
// forkresume's step 4.
// sr:proves session-resume/claude
func TestT017_83_ResumeFromAnotherDirectoryAsRecorded(t *testing.T) {
	type seen struct{ event, where, folder string }
	read := func(raw string, wantSession string) (out []seen) {
		for _, l := range strings.Split(raw, "\n") {
			var p map[string]any
			if json.Unmarshal([]byte(l), &p) != nil || p["session_id"] != wantSession {
				continue
			}
			cwd, _ := p["cwd"].(string)
			tp, _ := p["transcript_path"].(string)
			where := "root"
			if strings.HasSuffix(cwd, "/sub") {
				where = "sub"
			}
			folder := "original"
			if strings.Contains(filepath.Base(filepath.Dir(tp)), "-sub") {
				folder = "new"
			}
			out = append(out, seen{p["hook_event_name"].(string), where, folder})
		}
		return
	}
	rawReal, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/forkresume/samples/*/payloads.jsonl"))
	require.NoError(t, err)
	all := read(string(rawReal), "00000000-0000-4000-8000-0000000000a1")
	last := all[len(all)-4:]
	want := []seen{{"SessionStart", "sub", "new"}, {"UserPromptSubmit", "sub", "original"}, {"Stop", "sub", "original"}, {"SessionEnd", "sub", "original"}}
	require.Equal(t, want, last, "recorded")

	root := t.TempDir()
	cfg := filepath.Join(root, "config")
	first, second := filepath.Join(root, "first"), filepath.Join(root, "first", "sub")
	require.NoError(t, os.MkdirAll(second, 0o755))
	log := filepath.Join(root, "payloads.log")
	events := map[string]string{}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "Stop", "SessionEnd"} {
		events[ev] = payloadLogger(t, root, "log-"+ev+".sh", log, "")
	}
	settings(t, root, events)
	_ = first
	out, code := runInDir(t, first, nil, "--script", script(t, first, "a"), "--session-id", "00000000-0000-4000-8000-0000000000a1",
		"--project-dir", first, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	require.NoError(t, os.WriteFile(log, nil, 0o644))
	for _, d := range []string{first, second} {
		require.NoError(t, os.MkdirAll(filepath.Join(d, ".claude"), 0o755))
		b, _ := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
		require.NoError(t, os.WriteFile(filepath.Join(d, ".claude", "settings.json"), b, 0o644))
	}
	out, code = runInDir(t, second, nil, "--script", script(t, second, "b"), "--resume", "00000000-0000-4000-8000-0000000000a1",
		"--project-dir", second, "--config-dir", cfg, "-p", "again")
	require.Equal(t, 0, code, out)
	rawGot, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, want, read(string(rawGot), "00000000-0000-4000-8000-0000000000a1"))
}
