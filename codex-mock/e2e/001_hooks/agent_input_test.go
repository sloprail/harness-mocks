package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/agent-input-validation: the model calls spawn_agent
// with an empty argument object. The hooks.json is the run's own.

// dispatchScenario runs the mock on the recorded run's setup, with a script
// that makes the given spawn_agent calls (their inputs, one per line in
// $CALLS) and then ends.
func dispatchScenario(t *testing.T, setup string, inputs ...string) result {
	t.Helper()
	return execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(setup, "hook.sh"))},
		Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
input=$(sed -n "$((n+1))p" "$CALLS")
if [ -n "$input" ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_%s","name":"spawn_agent","input":%s}]}}\n' "$n" "$input"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`,
		Prompt: strings.TrimSpace(readFile(t, filepath.Join(setup, "prompt.txt"))),
		Env:    withCalls(t, inputs...),
	})
}

// A sub-agent dispatch lacking its message is refused with Codex's
// input-validation error, "Provide one of: message or items", and nothing
// runs: no sub-agent starts, the stream shows no tool item, no PostToolUse
// fires. Unlike Claude Code, Codex checks the input only after the PreToolUse
// hook has seen the call: the hook fires, with the empty input
// (runs/agent-input-validation). A dispatch with its message passes the check
// and fires both hooks.
// sr:proves agent-input-validation/codex
func TestDispatchWithoutMessageIsRefusedAfterThePreToolUseHook(t *testing.T) {
	run := filepath.Join(runsDir, "agent-input-validation")
	samples, err := filepath.Glob(filepath.Join(run, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	sample := samples[len(samples)-1]

	// what the recording shows: PreToolUse for spawn_agent with an empty
	// input and nothing after it but the Stop hook; the agent told the error
	var want []map[string]any
	for _, l := range jsonLines(readFile(t, filepath.Join(sample, "payloads.jsonl"))) {
		want = append(want, l)
	}
	require.Equal(t, []string{"PreToolUse", "Stop"}, []string{
		want[0]["hook_event_name"].(string), want[1]["hook_event_name"].(string)})
	require.Len(t, want, 2)
	require.Equal(t, "spawn_agent", want[0]["tool_name"])
	require.Equal(t, map[string]any{}, want[0]["tool_input"])
	transcripts, err := filepath.Glob(filepath.Join(sample, "transcript", "*.jsonl"))
	require.NoError(t, err)
	require.Contains(t, readFile(t, transcripts[0]), "Script error:\\nProvide one of: message or items")

	got := dispatchScenario(t, filepath.Join(run, "setup"), `{}`)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, sortedHookLines(want), sortedHookLines(got.hookLog()), "PreToolUse saw the empty call; no PostToolUse")
	assert.True(t, resultTold(t, got.rollout(t), "Provide one of: message or items", "sub-agents are not modelled"),
		"the agent is told the validation error")
	for _, e := range got.stream() {
		item, _ := e["item"].(map[string]any)
		assert.NotContains(t, []any{"command_execution", "collab_tool_call"}, item["type"], "nothing ran")
	}

	// a dispatch with its message passes the check, as in runs/agent-input-validation-spawn:
	// both hooks fire for it (the mock then answers it as an error: it runs no sub-agent)
	spawn := filepath.Join(runsDir, "agent-input-validation-spawn", "samples")
	spawned, err := filepath.Glob(filepath.Join(spawn, "*", "payloads.jsonl"))
	require.NoError(t, err)
	var seen []string
	for _, l := range jsonLines(readFile(t, spawned[len(spawned)-1])) {
		if l["tool_name"] == "spawn_agent" {
			seen = append(seen, l["hook_event_name"].(string))
		}
	}
	require.Equal(t, []string{"PreToolUse", "PostToolUse"}, seen)

	ok := dispatchScenario(t, filepath.Join(run, "setup"), `{"message":"hi"}`)
	require.Equal(t, 0, ok.Code, ok.Stderr)
	seen = nil
	for _, l := range ok.hookLog() {
		if l["tool_name"] == "spawn_agent" {
			seen = append(seen, l["hook_event_name"].(string))
		}
	}
	assert.Equal(t, []string{"PreToolUse", "PostToolUse"}, seen)
	assert.False(t, resultTold(t, ok.rollout(t), "Provide one of", "never"), "no validation error for a complete dispatch")
}
