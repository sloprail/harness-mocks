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

// TestT017_78_ResumeByNameAsRecorded: a session that was given a name (by
// --name, which the mock does not model; the real harness records it as a
// custom-title and an agent-name record, as the resume-name run's transcript
// shows) is resumed by `--resume <name>`: the same session continues in its own
// file, and the resume's SessionStart carries session_title, which an unnamed
// session's resume does not (forkresume).
// sr:proves session-resume/claude
func TestT017_78_ResumeByNameAsRecorded(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/resume-name/samples/*/events.jsonl"))
	require.NoError(t, err)
	var recordedTitle string
	for _, l := range strings.Split(string(raw), "\n") {
		var e struct {
			Hook    string
			Payload map[string]any
		}
		if json.Unmarshal([]byte(l), &e) == nil && e.Hook == "SessionStart" {
			assert.Equal(t, "resume", e.Payload["source"])
			recordedTitle, _ = e.Payload["session_title"].(string)
		}
	}
	require.Equal(t, "earlier-by-name", recordedTitle, "recorded: the resume carries the name")
	realRecs := readRecs(t, recordedFile(t, "../../snapshots/runs/resume-name/samples/*/transcript/*.jsonl"))
	require.Contains(t, realRecs[0].Raw, `"type":"custom-title"`)
	require.Contains(t, realRecs[1].Raw, `"type":"agent-name"`)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	run := func(args ...string) {
		out, code := runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "s"), "--project-dir", dir, "--config-dir", cfg, "-p", "go"}, args...)...)
		require.Equal(t, 0, code, out)
	}
	run("--session-id", "plain-1")
	run("--session-id", "named-1")
	// the records the real harness left in its named session, in the real order
	named := transcriptPath(t, cfg, dir, "named-1")
	body, err := os.ReadFile(named)
	require.NoError(t, err)
	head := `{"type":"custom-title","customTitle":"earlier-by-name","sessionId":"named-1"}` + "\n" +
		`{"type":"agent-name","agentName":"earlier-by-name","sessionId":"named-1"}` + "\n"
	require.NoError(t, os.WriteFile(named, append([]byte(head), body...), 0o644))
	run("--resume", "earlier-by-name")
	run("--resume", "plain-1")

	ps := payloads(t, log)
	require.Len(t, ps, 4)
	assert.Equal(t, "resume", ps[2]["source"])
	assert.Equal(t, "named-1", ps[2]["session_id"], "the name finds the named session")
	assert.Equal(t, recordedTitle, ps[2]["session_title"])
	assert.NotContains(t, ps[3], "session_title", "an unnamed session's resume carries no title")
	files, err := filepath.Glob(filepath.Join(filepath.Dir(named), "*.jsonl"))
	require.NoError(t, err)
	assert.Len(t, files, 2, "resuming by name made no new transcript")
	assert.Contains(t, readString(t, named), "go", "the turn went to the named session's file")
}
