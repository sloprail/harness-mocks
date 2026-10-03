package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/background-bash-start: the model started
// `sleep 25; echo BG-FINISHED > bg.out` with a yield time (exec_command), got
// back at once, and ran ls while the command was still running.

// yieldThenResult makes the calls of $CALLS, one per line as "<yield ms>
// <command>", each a Bash call with that yield_time_ms, then a final message.
const yieldThenResult = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
line=$(sed -n "$((n+1))p" "$CALLS")
if [ -n "$line" ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_%s","name":"Bash","input":{"command":%s,"yield_time_ms":%s}}]}}\n' "$n" "$(printf '%s' "${line#* }" | jq -Rs .)" "${line%% *}"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`

// recordedYields are the exec_command calls the model made, as the recorded
// rollout shows them: their commands and yield times.
func recordedYields(t *testing.T, rec recording) (cmds, yields []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	text := readFile(t, files[0])
	for _, m := range regexp.MustCompile(`cmd: \\"(.*?)\\",\\n\s*workdir: \\"[^"]*\\",\\n\s*yield_time_ms: (\d+)`).FindAllStringSubmatch(text, -1) {
		cmds, yields = append(cmds, strings.ReplaceAll(m[1], `\\\"`, `"`)), append(yields, m[2])
	}
	return
}

// recordedReceipt is the fields of what the recorded start call returned.
func recordedReceipt(t *testing.T, rec recording) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	m := regexp.MustCompile(`\{\\"start\\":(\{.*?\})\}`).FindStringSubmatch(readFile(t, files[0]))
	require.NotNil(t, m, "no start receipt in the recording")
	return receiptKeys(t, strings.ReplaceAll(m[1], `\"`, `"`))
}

func receiptKeys(t *testing.T, js string) (keys []string) {
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(js), &m), js)
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return
}

// A command started with a yield time that is still running when it is up is
// answered at once with a receipt: the session id of the running command and
// what it has printed so far, and no file its output goes to. It keeps running
// while the agent works (the next command runs beside it), shows on the stream
// as started and never completed, fires no PostToolUse, and ends with the run
// (recorded: runs/background-bash-start).
// sr:proves background-bash/codex
func TestACommandStillRunningAfterItsYieldTimeIsAnsweredWithAReceiptAndKeepsRunning(t *testing.T) {
	rec := loadRecording(t, "background-bash-start")
	cmds, yields := recordedYields(t, rec)
	require.Len(t, cmds, 2)
	require.Contains(t, cmds[0], "sleep 25")
	calls := []string{yields[0] + " " + strings.ReplaceAll(cmds[0], "sleep 25", "sleep 3"), yields[1] + " " + cmds[1]}
	f := filepath.Join(t.TempDir(), "calls")
	require.NoError(t, os.WriteFile(f, []byte(strings.Join(calls, "\n")+"\n"), 0o644))
	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    yieldThenResult,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       []string{"CALLS=" + f},
	})
	require.Equal(t, 0, got.Code, got.Stderr)

	// the stream: both commands started, only ls completed
	started, completed := map[string]string{}, map[string]map[string]any{}
	for _, e := range got.stream() {
		item, _ := e["item"].(map[string]any)
		if item["type"] != "command_execution" {
			continue
		}
		if e["type"] == "item.started" {
			started[item["id"].(string)] = innerCommand(item["command"].(string))
		} else {
			completed[item["id"].(string)] = item
		}
	}
	require.Len(t, started, 2)
	require.Len(t, completed, 1)
	for id, item := range completed {
		assert.Equal(t, cmds[1], started[id])
		assert.EqualValues(t, 0, item["exit_code"])
	}

	// PostToolUse for ls alone: the running command's comes when it ends
	var post []string
	for _, l := range got.hookLog() {
		if l["hook_event_name"] == "PostToolUse" {
			post = append(post, l["tool_input"].(map[string]any)["command"].(string))
		}
	}
	assert.Equal(t, []string{cmds[1]}, post)

	// the receipt is what the recording shows: a session id and what was
	// printed so far, no file; the ls result is plain
	told := toolOutputs(t, got.rollout(t))
	require.Len(t, told, 2)
	assert.Equal(t, recordedReceipt(t, rec), receiptKeys(t, told[0]))
	assert.Regexp(t, `"session_id":\d+`, told[0])
	assert.Equal(t, "hook.sh\n", told[1])

	// it ran while the agent worked, and died with the run: no bg.out
	_, err := os.Stat(filepath.Join(got.Repo, "bg.out"))
	assert.True(t, os.IsNotExist(err), fmt.Sprint(err))
}
