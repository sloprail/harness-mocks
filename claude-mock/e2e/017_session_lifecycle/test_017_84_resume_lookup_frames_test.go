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

// framesIDs are the session_id of the SessionStart hook frames and of the
// assistant frames of a stream-json output.
func framesIDs(t *testing.T, stream string) (hook, result []string) {
	t.Helper()
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil {
			continue
		}
		id, _ := f["session_id"].(string)
		switch {
		case f["type"] == "system" && (f["subtype"] == "hook_started" || f["subtype"] == "hook_response") && f["hook_event"] == "SessionStart":
			hook = append(hook, id)
		case f["type"] == "assistant":
			result = append(result, id)
		}
	}
	return
}

// A session found by name, path or --continue has its SessionStart hook frames
// stream under another id than the session's own, which init and the later frames
// carry (runs/resume-name, resume-path, resume-continue); a resume by the id
// itself keeps one id throughout (runs/forkresume).
// sr:proves session-resume/claude
func TestT017_84_ResumeLookupHookFramesCarryTheLookupsID(t *testing.T) {
	for _, run := range []string{"resume-name", "resume-path", "resume-continue"} {
		raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/"+run+"/samples/*/stream.jsonl"))
		require.NoError(t, err)
		hook, result := framesIDs(t, string(raw))
		require.NotEmpty(t, hook, run)
		require.NotEmpty(t, result, run)
		for _, id := range hook {
			assert.NotEqual(t, result[0], id, "recorded "+run)
		}
	}
	for name, args := range map[string]func(path string) []string{
		"path":     func(path string) []string { return []string{"--resume", path} },
		"continue": func(string) []string { return []string{"--continue"} },
		"id":       func(string) []string { return []string{"--resume", "earlier-1"} },
		"name": func(path string) []string { // the records the real harness leaves in a named session (runs/resume-name)
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			head := `{"type":"custom-title","customTitle":"by-name","sessionId":"earlier-1"}` + "\n" + `{"type":"agent-name","agentName":"by-name","sessionId":"earlier-1"}` + "\n"
			require.NoError(t, os.WriteFile(path, append([]byte(head), body...), 0o644))
			return []string{"--resume", "by-name"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", filepath.Join(dir, "p.log"), "")})
			out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a"), "--session-id", "earlier-1",
				"--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "one")
			require.Equal(t, 0, code, out)
			path := transcriptPath(t, cfg, dir, "earlier-1")
			out, code = runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "b"),
				"--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "two"}, args(path)...)...)
			require.Equal(t, 0, code, out)
			hook, result := framesIDs(t, out)
			require.NotEmpty(t, hook)
			require.NotEmpty(t, result)
			for _, id := range result {
				assert.Equal(t, "earlier-1", id, "the session itself")
			}
			for _, id := range hook {
				if name == "id" {
					assert.Equal(t, "earlier-1", id)
				} else {
					assert.NotEqual(t, "earlier-1", id, "the lookup's own id")
					assert.NotEmpty(t, id)
				}
			}
			ps := payloads(t, filepath.Join(dir, "p.log"))
			assert.Equal(t, "earlier-1", ps[len(ps)-1]["session_id"], "the hook's payload names the session itself")
		})
	}
}

// A fork's hooks name the fork's own transcript, on every event, not the file
// of the session it branched from (runs/forkresume: the fork's SessionStart,
// UserPromptSubmit, Stop and SessionEnd all carry the new session's file).
// sr:proves session-fork/claude
func TestT017_84_ForkHooksNameTheForksTranscript(t *testing.T) {
	dir := t.TempDir()
	cfg, log := filepath.Join(dir, "config"), filepath.Join(dir, "p.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "UserPromptSubmit": h, "Stop": h, "SessionEnd": h})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a"), "--session-id", "orig-1", "--project-dir", dir, "--config-dir", cfg, "-p", "one")
	require.Equal(t, 0, code, out)
	write(t, log, "", 0o644)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "b"), "--resume", "orig-1", "--fork-session", "--session-id", "fork-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "two")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 4)
	for _, p := range ps {
		assert.Equal(t, "fork-1", p["session_id"], p["hook_event_name"])
		assert.True(t, strings.HasSuffix(p["transcript_path"].(string), "/fork-1.jsonl"), "%v: %v", p["hook_event_name"], p["transcript_path"])
	}
}

// A10N_MOCK_NO_RESUME=1 makes a resume by name behave as an unknown session as
// well, and so does a resume that also forks.
// sr:proves session-resume-unknown/claude
// sr:invariant no-resume
func TestT017_84_NoResumeAppliesToNamesAndForks(t *testing.T) {
	for _, args := range [][]string{{"--resume", "some-name"}, {"--resume", "some-id", "--fork-session"}} {
		dir := t.TempDir()
		out, code := runInDir(t, dir, []string{"A10N_MOCK_NO_RESUME=1"}, append([]string{"--script", script(t, dir, "s"),
			"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "go"}, args...)...)
		assert.Equal(t, 1, code, out)
		assert.Contains(t, out, "No conversation found with session ID: ")
		assert.Contains(t, out, `"subtype":"error_during_execution"`)
	}
}
