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

// The PostToolUse payload also carries the turn's id and the call's id, and it
// is sent after a command that exits non-zero as after one that succeeds
// (hooks#posttooluse, runs/shell-exit-status). Codex's payload has no call
// duration: none of the recorded payloads holds one.
// sr:proves posttooluse-payload/codex
func TestPostToolUsePayloadHasTurnAndCallIDsAndFiresAfterFailure(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PostToolUse"),
		Files:     map[string]string{"hook.sh": logHook},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo FINE", "sh -c 'echo BAD; exit 3'"),
	})
	post := eventsOf(r, "PostToolUse")
	require.Len(t, post, 2)
	assert.Equal(t, "BAD\n", post[1]["tool_response"], "after a failing command too")
	for _, p := range post {
		assert.NotEmpty(t, p["turn_id"])
		assert.NotEmpty(t, p["tool_use_id"])
		assert.NotContains(t, p, "duration_ms")
	}
	assert.NotEqual(t, post[0]["tool_use_id"], post[1]["tool_use_id"])
	assert.Equal(t, post[0]["turn_id"], post[1]["turn_id"])

	// a recorded run's payloads hold the same keys, and no duration
	rec := loadRecording(t, "posttool-block")
	var recKeys []string
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if l["hook_event_name"] == "PostToolUse" {
			for k := range l {
				recKeys = append(recKeys, k)
			}
			assert.NotContains(t, l, "duration_ms")
		}
	}
	assert.Subset(t, recKeys, []string{"turn_id", "tool_use_id", "tool_name", "tool_input", "tool_response"})
	for k := range post[0] {
		assert.Contains(t, recKeys, k, "the mock's payload has no field the real one lacks")
	}
}

// A SessionEnd hook always runs to its end before the run exits, even when its
// handler is marked async (hooks#sessionend): the run waits for it.
// sr:proves session-end-hook/codex
func TestSessionEndHookRunsSynchronouslyEvenWhenAsync(t *testing.T) {
	hooks, err := json.Marshal(map[string]any{"hooks": map[string]any{"SessionEnd": []any{map[string]any{
		"hooks": []any{map[string]any{"type": "command", "command": "sh hook.sh", "async": true}}}}}})
	require.NoError(t, err)
	r := execMock(t, scenario{
		HooksJSON: string(hooks),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; sleep 0.5; echo ended >"$(dirname "$HOOK_LOG")/session-end-done"`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t),
	})
	require.Equal(t, 0, r.Code, r.Stderr)
	b, err := os.ReadFile(filepath.Join(r.Tmp, "session-end-done"))
	require.NoError(t, err, "the run exited before the hook finished")
	assert.Equal(t, "ended", strings.TrimSpace(string(b)))
}

// pairedToolUseIDs checks that every PostToolUse names the tool_use_id of the PreToolUse of its own
// call (the nearest earlier one with the same command), and that calls have ids of their own.
func pairedToolUseIDs(t *testing.T, log []map[string]any, who string) {
	t.Helper()
	pre := map[string]string{} // the command's last PreToolUse id
	seen := map[string]bool{}
	posts := 0
	for _, l := range log {
		in, _ := l["tool_input"].(map[string]any)
		cmd, _ := in["command"].(string)
		id, _ := l["tool_use_id"].(string)
		switch l["hook_event_name"] {
		case "PreToolUse":
			pre[cmd] = id
			seen[id] = true
		case "PostToolUse":
			posts++
			assert.NotEmpty(t, id, who)
			assert.Equal(t, pre[cmd], id, "%s: the PostToolUse of %q names its PreToolUse's tool_use_id", who, cmd)
		}
	}
	assert.NotZero(t, posts, who)
	assert.GreaterOrEqual(t, len(seen), 2, "%s: every call has an id of its own", who)
}

// PostToolUse carries the tool_use_id of the PreToolUse of the same call, as every recorded run shows
// (runs/file-tools, runs/hook-exit-codes), and the mock does the same.
// sr:proves posttooluse-payload/codex
func TestPostToolUseNamesThePreToolUseIDOfItsCall(t *testing.T) {
	for _, run := range []string{"file-tools", "hook-exit-codes"} {
		rec := loadRecording(t, run)
		pairedToolUseIDs(t, jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))), "recorded "+run)
	}
	got := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PreToolUse", "PostToolUse"),
		Files:     map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo one", "echo two"),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	pairedToolUseIDs(t, got.hookLog(), "mock")
}
