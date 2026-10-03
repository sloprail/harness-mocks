package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedResume is what the real harness did in a run that resumed one earlier
// session: the SessionStart source and whether its session id is the earlier
// session's, and the user prompts the one transcript file holds afterwards.
func recordedResume(t *testing.T, run string) (source string, prompts []string) {
	t.Helper()
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/"+run+"/samples/*/events.jsonl"))
	require.NoError(t, err)
	for _, l := range strings.Split(string(raw), "\n") {
		var e struct {
			Hook    string
			Payload map[string]any
		}
		if json.Unmarshal([]byte(l), &e) == nil && e.Hook == "SessionStart" {
			source, _ = e.Payload["source"].(string)
			assert.Equal(t, "<SESSION_ID>", e.Payload["session_id"])
		}
	}
	files, err := filepath.Glob("../../snapshots/runs/" + run + "/samples/*/transcript/*.jsonl")
	require.NoError(t, err)
	require.Len(t, files, 1, "one transcript file: the session continued in place")
	for _, r := range readRecs(t, files[0]) {
		if r.Type == "user" && strings.HasPrefix(string(r.Message), `{"role":"user","content":"Reply only`) {
			var m struct{ Content string }
			require.NoError(t, json.Unmarshal(r.Message, &m))
			prompts = append(prompts, m.Content)
		}
	}
	return
}

// TestT017_73_ResumeByPathAndContinueAsRecorded: `--resume <path of the
// transcript file>` and `--continue` (the directory's most recent session) each
// resume the earlier session, as the resume-path and resume-continue runs did:
// SessionStart source "resume" under the earlier session's id, and the new turn
// appended to the earlier session's own file, no new file.
// sr:proves session-resume/claude
func TestT017_73_ResumeByPathAndContinueAsRecorded(t *testing.T) {
	for _, tc := range []struct {
		run  string
		args func(path string) []string
	}{
		{"resume-path", func(path string) []string { return []string{"--resume", path} }},
		{"resume-continue", func(string) []string { return []string{"--continue"} }},
	} {
		t.Run(tc.run, func(t *testing.T) {
			source, prompts := recordedResume(t, tc.run)
			assert.Equal(t, "resume", source)
			assert.Equal(t, []string{"Reply only ONE", "Reply only TWO"}, prompts)

			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "payloads.log")
			settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
			out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a"), "--session-id", "earlier-1",
				"--project-dir", dir, "--config-dir", cfg, "-p", "Reply only ONE")
			require.Equal(t, 0, code, out)
			path := transcriptPath(t, cfg, dir, "earlier-1")
			out, code = runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "b"),
				"--project-dir", dir, "--config-dir", cfg, "-p", "Reply only TWO"}, tc.args(path)...)...)
			require.Equal(t, 0, code, out)

			ps := payloads(t, log)
			require.Len(t, ps, 2)
			assert.Equal(t, "resume", ps[1]["source"])
			assert.Equal(t, "earlier-1", ps[1]["session_id"], "the earlier session's own id")
			files, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.jsonl"))
			require.NoError(t, err)
			assert.Equal(t, []string{path}, files, "no new transcript: the session continues in its own file")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(raw), "Reply only ONE")
			assert.Contains(t, string(raw), "Reply only TWO")
		})
	}
}

// TestT017_74_ContinueTakesTheMostRecentSession: with several sessions in the
// directory --continue resumes the one written last.
// sr:proves session-resume/claude
func TestT017_74_ContinueTakesTheMostRecentSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	for _, id := range []string{"older", "newer"} {
		out, code := runInDir(t, dir, nil, "--script", script(t, dir, id), "--session-id", id,
			"--project-dir", dir, "--config-dir", cfg, "-p", "first "+id)
		require.Equal(t, 0, code, out)
	}
	older := transcriptPath(t, cfg, dir, "older")
	require.NoError(t, os.Chtimes(older, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)))
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "c"), "--continue",
		"--project-dir", dir, "--config-dir", cfg, "-p", "go on")
	require.Equal(t, 0, code, out)
	assert.Contains(t, readString(t, transcriptPath(t, cfg, dir, "newer")), "go on")
	assert.NotContains(t, readString(t, older), "go on")

}

// TestT017_88_ContinueWithNothingToContinueStartsANewSession: in a directory with
// no session, `--continue` is not an error: as in the continue-none run, a new
// session starts (SessionStart source "startup", the session's own new id).
// sr:proves session-resume/claude
func TestT017_88_ContinueWithNothingToContinueStartsANewSession(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/resume-continue-none/samples/*/events.jsonl"))
	require.NoError(t, err)
	var recorded []string
	for _, l := range strings.Split(string(raw), "\n") {
		var e struct {
			Hook    string
			Payload map[string]any
		}
		if json.Unmarshal([]byte(l), &e) == nil && e.Hook == "SessionStart" {
			recorded = append(recorded, e.Payload["source"].(string))
		}
	}
	require.Equal(t, []string{"startup"}, recorded, "recorded")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "d"), "--continue",
		"--project-dir", dir, "--config-dir", cfg, "-p", "go on")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Equal(t, "startup", ps[0]["source"])
	assert.NotEmpty(t, ps[0]["session_id"])
	assert.FileExists(t, transcriptPath(t, cfg, dir, ps[0]["session_id"].(string)))
}
