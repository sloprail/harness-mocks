package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/shell-exit-status: commands exiting 1 with no output,
// 1 with output, 2 with both streams, 3 with none, and 0, each followed by a
// PostToolUse.

// PostToolUse fires after every command, one that failed as well as one that
// succeeded, with the command as tool_input and its output as tool_response,
// the same payloads as the recording's; and, as in the recording, each names
// its call (tool_use_id, different for each) and its turn (turn_id, the same
// for all of them).
// sr:proves posttooluse-payload/codex
func TestPostToolUseFiresForEveryCommandNamingItsCallAndTurn(t *testing.T) {
	rec := loadRecording(t, "shell-exit-status")
	rec.calls = nil
	var recorded []map[string]any
	for _, p := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if in, ok := p["tool_input"].(map[string]any); ok && p["hook_event_name"] == "PostToolUse" {
			rec.calls = append(rec.calls, in["command"].(string))
			recorded = append(recorded, p)
		}
	}
	require.Len(t, rec.calls, 5)
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)

	var posted []map[string]any
	for _, l := range got.hookLog() {
		if l["hook_event_name"] == "PostToolUse" {
			posted = append(posted, l)
		}
	}
	require.Len(t, posted, 5)
	// (checked first: sortedHookLines normalizes the payloads it is given away)
	for name, payloads := range map[string][]map[string]any{"recording": recorded, "mock": posted} {
		calls, turns := map[any]bool{}, map[any]bool{}
		for _, p := range payloads {
			assert.NotEmpty(t, p["tool_use_id"], name)
			assert.NotEmpty(t, p["turn_id"], name)
			calls[p["tool_use_id"]], turns[p["turn_id"]] = true, true
		}
		assert.Len(t, calls, 5, "%s: a call id each", name)
		assert.Len(t, turns, 1, "%s: one turn for all of them", name)
	}

	assert.Equal(t, sortedHookLines(recorded), sortedHookLines(posted), "input and response of each, failed commands included")
}

// The recorded run runs/stops: three commands, the first two refused by a
// PreToolUse hook (JSON deny, exit 2), the third allowed.

// A command a before-tool hook refused does not run, so no PostToolUse fires
// for it: the only one is the allowed command's, with its input and output,
// as in the recording.
// sr:proves posttooluse-payload/codex
func TestNoPostToolUseForARefusedCommand(t *testing.T) {
	rec := loadRecording(t, "stops")
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)

	posted := func(lines []map[string]any) (out []map[string]any) {
		for _, l := range lines {
			if l["hook_event_name"] == "PostToolUse" {
				out = append(out, map[string]any{"input": l["tool_input"], "response": l["tool_response"]})
			}
		}
		return out
	}
	want := posted(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Equal(t, []map[string]any{{"input": map[string]any{"command": "echo FINE"}, "response": "FINE\n"}}, want)
	assert.Equal(t, want, posted(got.hookLog()))
}
