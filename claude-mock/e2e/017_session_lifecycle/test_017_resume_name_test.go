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

// framesBeforeInit lists the type/subtype of the system frames of a stream-json
// output up to the first init, hook frames and the title frame among them.
func framesBeforeInit(out string) (frames []string) {
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil || f["type"] != "system" {
			continue
		}
		if f["subtype"] == "init" {
			break
		}
		frames = append(frames, f["subtype"].(string))
		if f["subtype"] == "session_title_changed" {
			if f["title"] != "earlier-by-name" || f["session_id"] == nil {
				frames = append(frames, "!title")
			}
		}
	}
	return
}

// recordedFrames is the same list from the resume-name run's stream.
func recordedFrames(t *testing.T) []string {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/resume-name/samples/*/stream.jsonl"))
	require.NoError(t, err)
	return framesBeforeInit(string(raw))
}

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
// file, its SessionStart carries session_title and the stream a session_title_changed
// frame after the hooks' and before init, as recorded (the recorded UserPromptSubmit
// carries the title too, which the mock's does not: the cell's deviation), and an
// unnamed session's resume carries none.
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
	run := func(args ...string) string {
		out, code := runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "s"), "--project-dir", dir, "--config-dir", cfg, "-p", "go"}, args...)...)
		require.Equal(t, 0, code, out)
		return out
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
	stream := run("--resume", "earlier-by-name")
	run("--resume", "plain-1")

	events, titles := titlesOf(payloads(t, log)[before:], "named-1")
	assert.Equal(t, wantEvents, events, "the name finds the named session")
	assert.Equal(t, []string{"earlier-by-name", "", "", ""}, titles, "SessionStart carries the title (the UserPromptSubmit one is the deviation)")
	assert.Equal(t, recordedFrames(t), framesBeforeInit(stream), "the stream's frames up to init, as recorded")
	_, plainTitles := titlesOf(payloads(t, log), "plain-1")
	assert.Equal(t, make([]string, len(plainTitles)), plainTitles, "an unnamed session's hooks carry no title")
	files, err := filepath.Glob(filepath.Join(filepath.Dir(named), "*.jsonl"))
	require.NoError(t, err)
	assert.Len(t, files, 2, "resuming by name made no new transcript")
}
