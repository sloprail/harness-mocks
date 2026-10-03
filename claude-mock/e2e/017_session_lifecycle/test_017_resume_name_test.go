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

// titlesOf is each hook event's session_title (empty when the payload has none)
// in the payloads of one session, in order.
func titlesOf(ps []map[string]any, session string) (events, titles []string) {
	for _, p := range ps {
		if p["session_id"] != session {
			continue
		}
		title, _ := p["session_title"].(string)
		events, titles = append(events, p["hook_event_name"].(string)), append(titles, title)
	}
	return
}

// TestT017_78_ResumeByNameAsRecorded: a session that was given a name (by
// --name, which the mock does not model; the real harness records it as a
// custom-title and an agent-name record, as the resume-name run's transcript
// shows) is resumed by `--resume <name>`: the same session continues in its own
// file, and the resume's SessionStart and UserPromptSubmit carry session_title
// (its Stop and SessionEnd do not), which an unnamed session's resume never does.
// sr:proves session-resume/claude
func TestT017_78_ResumeByNameAsRecorded(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/resume-name/samples/*/payloads.jsonl"))
	require.NoError(t, err)
	var recordedPayloads []map[string]any
	for _, l := range strings.Split(string(raw), "\n") {
		var p map[string]any
		if json.Unmarshal([]byte(l), &p) == nil && p["hook_event_name"] != nil {
			recordedPayloads = append(recordedPayloads, p)
		}
	}
	recEvents, recTitles := titlesOf(recordedPayloads, recordedPayloads[0]["session_id"].(string))
	wantEvents := []string{"SessionStart", "UserPromptSubmit", "Stop", "SessionEnd"}
	wantTitles := []string{"earlier-by-name", "earlier-by-name", "", ""}
	require.Equal(t, wantEvents, recEvents)
	require.Equal(t, wantTitles, recTitles, "recorded: the title is on the resume's SessionStart and UserPromptSubmit only")
	realRecs := readRecs(t, recordedFile(t, "../../snapshots/runs/resume-name/samples/*/transcript/*.jsonl"))
	require.Contains(t, realRecs[0].Raw, `"type":"custom-title"`)
	require.Contains(t, realRecs[1].Raw, `"type":"agent-name"`)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "UserPromptSubmit": h, "Stop": h, "SessionEnd": h})
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
	before := len(payloads(t, log))
	run("--resume", "earlier-by-name")
	run("--resume", "plain-1")

	events, titles := titlesOf(payloads(t, log)[before:], "named-1")
	assert.Equal(t, wantEvents, events, "the name finds the named session")
	assert.Equal(t, wantTitles, titles)
	_, plainTitles := titlesOf(payloads(t, log), "plain-1")
	assert.Equal(t, make([]string, len(plainTitles)), plainTitles, "an unnamed session's hooks carry no title")
	files, err := filepath.Glob(filepath.Join(filepath.Dir(named), "*.jsonl"))
	require.NoError(t, err)
	assert.Len(t, files, 2, "resuming by name made no new transcript")
}
