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

// The recorded runs runs/file-tools (a missing file read, a file created, read
// back and replaced) and runs/file-tools-failure (a read of a missing file and
// a patch updating one).

// patchScript makes the calls of $CALLS, one JSON object per line ({name,
// input}), then a final message.
const patchScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
call=$(sed -n "$((n+1))p" "$CALLS")
if [ -n "$call" ]; then
  printf '%s\n' "$call" | jq -c --arg id "call_$n" '{type:"assistant",message:{content:[{type:"tool_use",id:$id,name:.name,input:.input}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`

type toolCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// recordedToolCalls are the calls the model made in a recorded run, as its
// hooks saw them, with the run's directory as dir.
func recordedToolCalls(t *testing.T, rec recording, dir string) (calls []toolCall) {
	t.Helper()
	seen := map[string]bool{}
	for _, p := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		id, _ := p["tool_use_id"].(string)
		if p["hook_event_name"] != "PreToolUse" || seen[id] {
			continue
		}
		seen[id] = true
		in := p["tool_input"].(map[string]any)
		calls = append(calls, toolCall{p["tool_name"].(string),
			map[string]any{"command": strings.ReplaceAll(in["command"].(string), "<RUN>", dir)}})
	}
	return calls
}

// replayCalls runs the mock on a recorded run's setup, making the given calls.
func replayCalls(t *testing.T, rec recording, calls func(repo string) []toolCall) result {
	t.Helper()
	// the repository is made by execMock, so the calls name it through $REPO:
	// the script expands it
	var lines []string
	for _, c := range calls("$REPO") {
		b, err := json.Marshal(c)
		require.NoError(t, err)
		lines = append(lines, string(b))
	}
	f := filepath.Join(t.TempDir(), "calls")
	require.NoError(t, os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	return execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    strings.Replace(patchScript, `call=$(sed -n "$((n+1))p" "$CALLS")`, `call=$(sed -n "$((n+1))p" "$CALLS" | sed "s#\$REPO#$(pwd)#g")`, 1),
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       []string{"CALLS=" + f},
	})
}

// fileChanges are the file_change items of a stream, paths made relative to dir.
func fileChanges(events []map[string]any, dir string) (out []string) {
	for _, e := range events {
		if item, _ := e["item"].(map[string]any); item != nil && item["type"] == "file_change" {
			for _, ch := range item["changes"].([]any) {
				c := ch.(map[string]any)
				out = append(out, e["type"].(string)+" "+item["status"].(string)+" "+
					c["kind"].(string)+" "+strings.ReplaceAll(c["path"].(string), dir, "<RUN>"))
			}
		}
	}
	return
}

// The agent reads and writes files in its working directory the way Codex does:
// it has no tool to read a file, so it reads through the shell (the output is
// the file's content), and it writes with apply_patch, whose "Add File" creates
// a file and whose "Update File" changes one; the stream shows each patch as a
// file_change item (kind add, update), PostToolUse sees the patch as the
// command and a success report as the response, and the agent is told {}
// (runs/file-tools). The file written is on disk, as the patch left it.
// sr:proves file-tools/codex
func TestFilesAreReadByShellAndWrittenByPatch(t *testing.T) {
	rec := loadRecording(t, "file-tools")
	got := replayCalls(t, rec, func(repo string) []toolCall { return recordedToolCalls(t, rec, repo) })
	require.Equal(t, 0, got.Code, got.Stderr)

	recStream := jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl")))
	want := fileChanges(recStream, "<RUN>")
	require.Len(t, want, 4)
	assert.Equal(t, want, fileChanges(got.stream(), got.Repo), "the patches, as the stream shows them")

	// the shell reads: the file's content back, nothing for the file not there
	_, wantExits := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}.commands()
	_, gotExits := got.commands()
	assert.Equal(t, wantExits, gotExits)
	var readOut []any
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "command_execution" {
			readOut = append(readOut, item["aggregated_output"])
		}
	}
	assert.Equal(t, []any{"", "HELLO-FILE\n"}, readOut)

	b, err := os.ReadFile(filepath.Join(got.Repo, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "HELLO-AGAIN\n", string(b), "the patch replaced the file")

	// the hooks saw the same tool names, inputs and responses as recorded
	var gotLines []map[string]any
	for _, l := range got.hookLog() {
		raw, _ := json.Marshal(l)
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(string(raw), got.Repo, "<RUN>")), &m))
		gotLines = append(gotLines, m)
	}
	assert.Equal(t, sortedHookLines(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))), sortedHookLines(gotLines))

	assert.True(t, resultTold(t, got.rollout(t), "{}", "Success"), "the agent is told {} of a patch")
}

// A call that fails is told to the agent as an error: a shell read of a file
// that is not there prints the shell's complaint and fails the command (the
// stream's item has status failed and exit 1), and a patch updating a file
// that is not there is refused with the error naming the file, writing nothing
// (runs/file-tools-failure).
// sr:proves file-tools/codex
func TestFailedFileCallsAreErrors(t *testing.T) {
	rec := loadRecording(t, "file-tools-failure")
	patch := "*** Begin Patch\n*** Update File: nothere2.txt\n@@\n-OLD\n+NEW\n*** End Patch"
	got := replayCalls(t, rec, func(string) []toolCall {
		return []toolCall{{"Bash", map[string]any{"command": "cat ./nothere.txt"}},
			{"apply_patch", map[string]any{"command": patch}}}
	})
	require.Equal(t, 0, got.Code, got.Stderr)

	var items []map[string]any
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "command_execution" {
			items = append(items, item)
		}
	}
	recItems := jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl")))
	var want map[string]any
	for _, e := range recItems {
		if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "command_execution" {
			want = item
		}
	}
	require.Len(t, items, 1)
	assert.Equal(t, "failed", want["status"])
	assert.Equal(t, want["status"], items[0]["status"])
	assert.Equal(t, want["exit_code"], items[0]["exit_code"])
	assert.Contains(t, items[0]["aggregated_output"], "nothere.txt: No such file or directory")

	// the failed shell read fired both hooks, the response being the shell's complaint
	bash := func(lines []map[string]any) (out []map[string]any) {
		for _, l := range lines {
			if l["tool_name"] == "Bash" {
				out = append(out, l)
			}
		}
		return
	}
	wantHooks := bash(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Len(t, wantHooks, 2)
	assert.Equal(t, sortedHookLines(wantHooks), sortedHookLines(bash(got.hookLog())))

	rollout := got.rollout(t)
	assert.True(t, resultTold(t, rollout, "apply_patch verification failed: Failed to read file to update "+got.Repo+"/nothere2.txt: No such file or directory", "{}"))
	assert.Empty(t, fileChanges(got.stream(), got.Repo), "a patch that fails changes no file")
	_, err := os.Stat(filepath.Join(got.Repo, "nothere2.txt"))
	assert.True(t, os.IsNotExist(err))
}
