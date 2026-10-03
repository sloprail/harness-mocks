package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded runs runs/task-notifications-bg and runs/task-notifications-inturn:
// a Shell left running in the background (block_until_ms 0), and the agent told
// when it finished.

// bgRun is what a replayed run showed: its stream, its hook payloads, and what
// the scenario script was asked on each of its runs ("<prompt>|<tool calls so
// far>").
type bgRun struct {
	frames []map[string]any
	hooks  []map[string]any
	asked  []string
}

// bgKinds names the frames that place a notification in a run: the end of a
// tool call, the notification, the agent's text and the result.
func bgKinds(frames []map[string]any) (out []string) {
	for _, f := range frames {
		typ, _ := f["type"].(string)
		sub, _ := f["subtype"].(string)
		switch {
		case typ == "tool_call" && sub == "completed", typ == "system" && sub == "task_notification", typ == "result":
			out = append(out, typ+"/"+sub)
		}
	}
	return out
}

// recordedStream is the stream of the run's newest sample.
func recordedStream(t *testing.T, run string) []map[string]any {
	t.Helper()
	samples, err := filepath.Glob(filepath.Join("..", "..", "snapshots", "runs", run, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	sort.Strings(samples)
	return readJSONL(t, filepath.Join(samples[len(samples)-1], "stream.jsonl"))
}

// replayBackground plays the recorded run's tool calls against the mock, the
// agent saying says[0] first and says[1] when it is handed the notification's
// turn.
func replayBackground(t *testing.T, run string) bgRun {
	t.Helper()
	setup, _, calls, prompt := recording(t, run)
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	say := func(text string) string {
		return jsonString(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": text}}}})
	}
	for i, c := range calls {
		var f map[string]any
		require.NoError(t, json.Unmarshal([]byte(c), &f))
		args := f["tool_call"].(map[string]any)["shellToolCall"].(map[string]any)["args"].(map[string]any)
		input := map[string]any{"command": args["command"]}
		if bg, _ := args["isBackground"].(bool); bg {
			input["block_until_ms"] = 0
			if d, ok := args["description"]; ok {
				input["description"] = d
			}
		}
		line := jsonString(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "tool_use", "id": f["call_id"], "name": "Bash", "input": input}}}})
		require.NoError(t, os.WriteFile(filepath.Join(scratch, itoa(i)+".json"), []byte(line+"\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "end.json"), []byte(say("LAUNCHED")+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "told.json"), []byte(say("FINISHED")+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
n=${n:-0}
if grep -q 'Briefly inform the user about the task result' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  echo "told|$n" >> "`+scratch+`/asked.log"
  cat "`+scratch+`/told.json"; exit 0
fi
echo "not told|$n" >> "`+scratch+`/asked.log"
f="`+scratch+`/$n.json"
[ -f "$f" ] || f="`+scratch+`/end.json"
cat "$f"
`), 0o755))
	logPath := filepath.Join(scratch, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", prompt)
	cmd.Dir = ws
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "HOOK_LOG=" + logPath, "TMPDIR=" + scratch, "A10N_MOCK_SCRIPT=" + script}
	out, err := cmd.Output()
	require.NoError(t, err, "the mock failed: %s", out)

	var got bgRun
	for _, l := range strings.Split(string(out), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			got.frames = append(got.frames, f)
		}
	}
	got.hooks = readJSONL(t, logPath)
	asked, err := os.ReadFile(filepath.Join(scratch, "asked.log"))
	require.NoError(t, err)
	got.asked = strings.Split(strings.TrimSpace(string(asked)), "\n")
	return got
}

// notificationOf is the run's task_notification frame.
func notificationOf(t *testing.T, frames []map[string]any) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, f := range frames {
		if f["type"] == "system" && f["subtype"] == "task_notification" {
			found = append(found, f)
		}
	}
	require.Len(t, found, 1)
	return found[0]
}

// TestAShellLeftInTheBackgroundIsAnsweredWithItsIdAndTellsTheAgentWhenItEnds:
// recorded, a Shell with block_until_ms 0 is answered at once with its shell id
// and pid (postToolUse's tool_output; no afterShellExecution), the session does
// not end while it runs, and when it ends a task_notification frame (its id, its
// status, its title) goes on the stream and the agent is given a turn of its
// own: the "Briefly inform the user about the task result" prompt. Replayed
// from runs/task-notifications-bg, where the agent has ended its turn.
// sr:proves task-notifications/cursor
func TestAShellLeftInTheBackgroundIsAnsweredWithItsIdAndTellsTheAgentWhenItEnds(t *testing.T) {
	t.Parallel()
	got := replayBackground(t, "task-notifications-bg")
	want := recordedStream(t, "task-notifications-bg")
	require.Equal(t, bgKinds(want), bgKinds(got.frames), "tool call, notification, result: the recorded order")

	note, recorded := notificationOf(t, got.frames), notificationOf(t, want)
	require.Equal(t, recorded["status"], note["status"])
	require.Equal(t, recorded["title"], note["title"], "a shell with no description is titled by its command")

	var out map[string]any
	for _, h := range got.hooks {
		require.NotEqual(t, "afterShellExecution", h["hook_event_name"], "a background shell has no end of its own to report")
		if h["hook_event_name"] == "postToolUse" {
			require.NoError(t, json.Unmarshal([]byte(h["tool_output"].(string)), &out))
		}
	}
	require.Contains(t, out, "pid")
	require.Equal(t, note["task_id"], itoa(int(out["shell_id"].(float64))), "the notification names the shell the call answered with")

	require.Equal(t, "told", strings.Split(got.asked[len(got.asked)-1], "|")[0], "the agent was given the notification's turn: %v", got.asked)
	require.Equal(t, "result/success", bgKinds(got.frames)[len(bgKinds(got.frames))-1])
	require.Equal(t, "FINISHED", textOfLast(got.frames), "the agent answered the notification's turn")
}

// TestAShellEndingWhileTheAgentWorksIsHandedOverOnlyAfterItsTurn: recorded, a
// shell that ended seconds before a foreground call did (2s against 12s) is
// not handed to the agent after that call's result: its notification comes
// after the agent's turn has ended, as a turn of its own (deviation of the
// capability: Cursor does not tell the agent inside the turn).
// sr:proves task-notifications/cursor
func TestAShellEndingWhileTheAgentWorksIsHandedOverOnlyAfterItsTurn(t *testing.T) {
	t.Parallel()
	got := replayBackground(t, "task-notifications-inturn")
	want := recordedStream(t, "task-notifications-inturn")
	require.Equal(t, bgKinds(want), bgKinds(got.frames))
	require.Equal(t, []string{"tool_call/completed", "tool_call/completed", "system/task_notification", "result/success"}, bgKinds(got.frames))
	require.Equal(t, "Background sleep 2 then BGDONE", notificationOf(t, got.frames)["title"])

	// the script was asked once per step: at 0 and 1 calls made, after the second
	// call, and then for the notification's turn with both calls behind it
	var told []string
	for _, a := range got.asked {
		told = append(told, a[strings.LastIndex(a, "|")+1:])
	}
	require.Equal(t, []string{"0", "1", "2", "2"}, told, got.asked)
	require.True(t, strings.HasPrefix(got.asked[3], "told"), "only the last step is the notification turn: %v", got.asked)
	for _, a := range got.asked[:3] {
		require.True(t, strings.HasPrefix(a, "not told"), "not told inside the turn: %v", got.asked)
	}
}

func textOfLast(frames []map[string]any) string {
	last := ""
	for _, f := range frames {
		if f["type"] != "assistant" {
			continue
		}
		msg, _ := f["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			if m, ok := b.(map[string]any); ok && m["type"] == "text" {
				last, _ = m["text"].(string)
			}
		}
	}
	return last
}
