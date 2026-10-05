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
