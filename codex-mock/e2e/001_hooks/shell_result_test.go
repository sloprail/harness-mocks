package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/shell-exit-status: commands exiting 1 with no output,
// 1 with output, 2 with both streams, 3 with none, and 0.

// A shell command's result is its output and a structured form of it, the
// stream's command_execution item: its exit code, what the command printed
// (both streams, interleaved) and a status, "failed" for any non-zero exit
// (grep finding nothing included) and "completed" for 0. The agent is told
// the output alone, the same for a failure as for a success: no exit code,
// no error mark (runs/shell-exit-status).
// sr:proves bash-tool-result/codex
func TestShellResultIsOutputAndItsStructuredForm(t *testing.T) {
	rec := loadRecording(t, "shell-exit-status")
	// the commands the model asked for, as the recorded hooks saw them
	rec.calls = nil
	for _, p := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if in, ok := p["tool_input"].(map[string]any); ok && p["hook_event_name"] == "PostToolUse" {
			rec.calls = append(rec.calls, in["command"].(string))
		}
	}
	require.Len(t, rec.calls, 5)
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)

	items := func(r result) (out []map[string]any) {
		for _, e := range r.stream() {
			if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "command_execution" {
				out = append(out, map[string]any{
					"output": item["aggregated_output"], "exit": item["exit_code"], "status": item["status"]})
			}
		}
		return
	}
	want := items(result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))})
	require.Len(t, want, 5)
	assert.Equal(t, want, items(got))

	// PostToolUse fires after every one of them, failed or not, with the
	// output as the response: the same payloads the recording holds
	var wantPost, gotPost []map[string]any
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if l["hook_event_name"] == "PostToolUse" {
			wantPost = append(wantPost, l)
		}
	}
	for _, l := range got.hookLog() {
		if l["hook_event_name"] == "PostToolUse" {
			gotPost = append(gotPost, l)
		}
	}
	require.Len(t, wantPost, 5)
	assert.Equal(t, sortedHookLines(wantPost), sortedHookLines(gotPost))

	byExit := map[float64]string{}
	for _, it := range items(got) {
		byExit[it["exit"].(float64)] = it["status"].(string)
	}
	assert.Equal(t, map[float64]string{0: "completed", 1: "failed", 2: "failed", 3: "failed"}, byExit)

	rollout := got.rollout(t)
	assert.True(t, resultTold(t, rollout, "MORE-OUTPUT\nERR-OUTPUT", "xit code"), "the output of a failed command, and no exit code")
	assert.True(t, resultTold(t, rollout, "FINE", "xit code"))

	// what the agent is told, command by command: the output alone, so a command that printed nothing
	// (grep finding nothing, exit 1; sh -c 'exit 3') is told an empty output, and no exit code
	printed := func(rollout string) (out []string) {
		for _, o := range toolOutputs(t, rollout) {
			if _, after, found := strings.Cut(o, "Output:\n"); found { // the real harness frames the output
				o = after
			}
			out = append(out, o)
			assert.NotContains(t, o, "xit code")
		}
		return
	}
	wantTold := printed(recordedRollout(t, rec))
	assert.Equal(t, []string{"", "SOME-OUTPUT\n", "MORE-OUTPUT\nERR-OUTPUT\n", "", "FINE\n"}, wantTold, "recorded")
	assert.Equal(t, wantTold, printed(rollout), "the mock's")
}
