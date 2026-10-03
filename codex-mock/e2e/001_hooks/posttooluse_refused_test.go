package e2e

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commandsByEvent are the shell commands the hook payloads name for an event, sorted.
func commandsByEvent(lines []map[string]any, event string) (out []string) {
	for _, l := range lines {
		if in, ok := l["tool_input"].(map[string]any); ok && l["hook_event_name"] == event {
			out = append(out, in["command"].(string))
		}
	}
	sort.Strings(out)
	return out
}

// PostToolUse fires only for a call that ran: the recorded run has a PreToolUse
// for every command, the ones a hook refused (a deny, an exit 2) included, and a
// PostToolUse for the one that ran (runs/stops), and the mock fires the same.
// sr:proves posttooluse-payload/codex
func TestPostToolUseFiresOnlyForACallThatRan(t *testing.T) {
	rec := loadRecording(t, "stops")
	want := jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	require.Greater(t, len(commandsByEvent(want, "PreToolUse")), len(commandsByEvent(want, "PostToolUse")),
		"the recording has refused calls")

	got := replay(t, rec).hookLog()
	assert.Equal(t, commandsByEvent(want, "PreToolUse"), commandsByEvent(got, "PreToolUse"))
	assert.Equal(t, commandsByEvent(want, "PostToolUse"), commandsByEvent(got, "PostToolUse"))
}
