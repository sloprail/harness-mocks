package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These pin the background-task lifecycle and the hook records to claude
// 2.1.282: controlled `claude -p` runs of it, its binary, and the real
// transcripts on one machine (claude-mock/EVIDENCE.md).

// messageText is a record's message.content when it is a string.
func messageText(r rec) string {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(r.Message, &m) != nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(m.Content, &s)
	return s
}

// toolResultOf is the tool_result block answering id in recs, and its record.
func toolResultOf(t *testing.T, recs []rec, id string) (map[string]any, rec) {
	t.Helper()
	for _, r := range recs {
		var m struct {
			Content []map[string]any `json:"content"`
		}
		if r.Type != "user" || json.Unmarshal(r.Message, &m) != nil {
			continue
		}
		for _, b := range m.Content {
			if b["type"] == "tool_result" && b["tool_use_id"] == id {
				return b, r
			}
		}
	}
	t.Fatalf("no tool_result for %s", id)
	return nil, rec{}
}

func indexWhere(recs []rec, from int, f func(rec) bool) int {
	for i := from; i < len(recs); i++ {
		if f(recs[i]) {
			return i
		}
	}
	return -1
}

func isStopSummary(r rec) bool { return r.Subtype == "stop_hook_summary" }

// TestT017_11_BackgroundBashFinishedMidTurn: a background Bash is answered at
// once with the real receipt; when it finishes while the agent is still
// working, the notification is handed over INSIDE the turn — a queued_command
// attachment (commandMode task-notification) after the next tool result — and
// UserPromptSubmit fires with it. Stop fires after, at the end of the turn.
// sr:proves background-bash/claude
// sr:proves task-notifications/claude
// sr:proves user-prompt-submit-hook/claude
func TestT017_11_BackgroundBashFinishedMidTurn(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	tmp := filepath.Join(dir, "tmp")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"UserPromptSubmit": h, "Stop": h})
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"echo BG-OUTPUT-4411; touch bgdone","description":"make output","run_in_background":true}`),
		// The foreground call outlasts the background one, whatever the machine's speed.
		toolUse("fg1", "Bash", `{"command":"while [ ! -f bgdone ]; do sleep 0.05; done; sleep 1"}`),
	)
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_TMPDIR=" + tmp}, "--script", sc, "--session-id", "bg-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "bg-1"))
	block, receipt := toolResultOf(t, recs, "bg1turn-s-a")
	id, _ := receipt.ToolUseResult["backgroundTaskId"].(string)
	require.Regexp(t, `^b[a-z0-9]{8}$`, id)
	root, err := filepath.EvalSymlinks(filepath.Join(tmp, "claude-"+strconv.Itoa(os.Getuid())))
	require.NoError(t, err)
	outFile := filepath.Join(root, encode(t, dir), "bg-1", "tasks", id+".output")
	assert.Equal(t, "Command running in background with ID: "+id+". Output is being written to: "+outFile+
		". You will be notified when it completes. To check interim output, use Read on that file path.", block["content"])
	assert.NotContains(t, receipt.ToolUseResult, "backgroundCwdHint", "no cd, no cwd note")
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "BG-OUTPUT-4411\n\n[exited with code 0]\n", string(data))

	_, fg := toolResultOf(t, recs, "fg1turn-s-b")
	fgAt := indexWhere(recs, 0, func(r rec) bool { return r.UUID == fg.UUID })
	q := recs[fgAt+1]
	require.Equal(t, "attachment", q.Type, "the notification is handed over right after the tool result it arrived during")
	assert.Equal(t, "queued_command", q.Attachment["type"])
	assert.Equal(t, "task-notification", q.Attachment["commandMode"])
	note := "<task-notification>\n<task-id>" + id + "</task-id>\n<tool-use-id>bg1turn-s-a</tool-use-id>\n<output-file>" + outFile +
		"</output-file>\n<status>completed</status>\n<summary>Background command \"make output\" completed (exit code 0)</summary>\n</task-notification>"
	assert.Equal(t, note, q.Attachment["prompt"])
	stopAt := indexWhere(recs, 0, isStopSummary)
	assert.Greater(t, stopAt, fgAt+1, "Stop fires after, at the end of the turn")
	assert.Equal(t, -1, indexWhere(recs, stopAt+1, isStopSummary), "and once: nothing was left to hand over")

	var prompts []any
	var stops []map[string]any
	for _, p := range payloads(t, log) {
		switch p["hook_event_name"] {
		case "UserPromptSubmit":
			prompts = append(prompts, p["prompt"])
		case "Stop":
			stops = append(stops, p)
		}
	}
	assert.Equal(t, []any{"hello", note}, prompts, "UserPromptSubmit fires for the handed-over notification")
	require.Len(t, stops, 1)
	assert.Equal(t, []any{}, stops[0]["background_tasks"])
}

// TestT017_11b_BackgroundBashFailure: a background Bash that exits non-zero is
// announced as failed: `Background command "<description>" failed with exit
// code N`.
func TestT017_11b_BackgroundBashFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"echo x; touch bgdone; exit 3","description":"fail","run_in_background":true}`),
		toolUse("fg1", "Bash", `{"command":"while [ ! -f bgdone ]; do sleep 0.05; done; sleep 1"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bg-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "bg-2"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Attachment["type"] == "queued_command" })
	require.GreaterOrEqual(t, at, 0)
	prompt := recs[at].Attachment["prompt"].(string)
	assert.Contains(t, prompt, "<status>failed</status>")
	assert.Contains(t, prompt, `<summary>Background command "fail" failed with exit code 3</summary>`)
}

// TestT017_11c_BackgroundBashStillRunningIsStopped: in a `claude -p` session,
// a background Bash still running when the turn ends is listed in Stop's
// background_tasks, then killed: its stopped notification goes to the output
// stream only, never the transcript, and nothing waits for it. A command that
// changes directory gets the cwd note (claude 2.1.282).
// sr:proves background-bash-reaped-at-exit/claude
// sr:proves stop-hook-payload/claude
func TestT017_11c_BackgroundBashStillRunningIsStopped(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"Stop": payloadLogger(t, dir, "log.sh", log, "")})
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"cd / && sleep 30","description":"long","run_in_background":true}`),
	)
	started := time.Now()
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bg-3",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Less(t, time.Since(started), 15*time.Second, "nothing waits for the command")

	recs := readRecs(t, transcriptPath(t, cfg, dir, "bg-3"))
	block, receipt := toolResultOf(t, recs, "bg1turn-s-a")
	id := receipt.ToolUseResult["backgroundTaskId"].(string)
	resolved, _ := filepath.EvalSymlinks(dir)
	hint := "Session cwd remains " + resolved + "; directory changes made by the backgrounded command do not apply to subsequent commands."
	assert.True(t, strings.HasSuffix(block["content"].(string), "on that file path.\n"+hint), block["content"])
	assert.Equal(t, hint, receipt.ToolUseResult["backgroundCwdHint"])

	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Equal(t, []any{map[string]any{"id": id, "type": "shell", "status": "running", "description": "long", "command": "cd / && sleep 30"}},
		ps[0]["background_tasks"])
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "bg-3"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "task-notification", "no notification is written for the stopped command")
	var frame map[string]any
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, `"subtype":"task_notification"`) {
			require.NoError(t, json.Unmarshal([]byte(l), &frame))
		}
	}
	require.NotNil(t, frame, "the stream carries the stopped notification")
	assert.Equal(t, "stopped", frame["status"])
	assert.Equal(t, id, frame["task_id"])
	assert.Equal(t, "long", frame["summary"])
}

// framesOf is the stream's task_* system frames for taskID, in order.
func framesOf(t *testing.T, out, taskID string) []map[string]any {
	t.Helper()
	var frames []map[string]any
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, `"subtype":"task_`) {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["task_id"] == taskID {
			frames = append(frames, m)
		}
	}
	return frames
}

// TestT017_12d_BackgroundSubAgentsOwnBash: a foreground Bash run by a
// background sub-agent streams task_started {owned_by_subagent, is_backgrounded
// false, task_type local_bash} and task_notification {status completed,
// output_file "", summary: its description} (F:bgagent). The root's own
// foreground Bash streams no task frame.
// sr:proves task-stream-frames/claude
func TestT017_12d_BackgroundSubAgentsOwnBash(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := script(t, dir, "sub", toolUse("sb1", "Bash", `{"command":"echo SUB","description":"sub step"}`))
	sc := script(t, dir, "s",
		toolUse("r1", "Bash", `{"command":"echo ROOT","description":"root step"}`),
		toolUse("ag1", "Agent", `{"prompt":"go","description":"bg","script":"`+sub+`","run_in_background":true}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "ob-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	var owned []map[string]any
	for _, l := range strings.Split(out, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil || m["type"] != "system" {
			continue
		}
		tid, _ := m["tool_use_id"].(string)
		if strings.HasPrefix(tid, "sb1") {
			owned = append(owned, m)
		}
		assert.False(t, strings.HasPrefix(tid, "r1"), "no frame for the root's own foreground Bash")
	}
	require.Len(t, owned, 2)
	assert.Equal(t, "task_started", owned[0]["subtype"])
	assert.Equal(t, true, owned[0]["owned_by_subagent"])
	assert.Equal(t, false, owned[0]["is_backgrounded"])
	assert.Equal(t, "local_bash", owned[0]["task_type"])
	assert.Equal(t, "task_notification", owned[1]["subtype"])
	assert.Equal(t, "completed", owned[1]["status"])
	assert.Equal(t, "", owned[1]["output_file"])
	assert.Equal(t, "sub step", owned[1]["summary"])
	assert.Equal(t, owned[0]["task_id"], owned[1]["task_id"])
}
