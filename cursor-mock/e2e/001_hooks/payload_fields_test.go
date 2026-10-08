package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordedPayloads are the payloads of a run's newest sample whose hook event is
// the named one.
func recordedPayloads(t *testing.T, run, event string) (out []map[string]any) {
	t.Helper()
	for _, h := range readJSONL(t, filepath.Join(newestSample(t, run), "payloads.jsonl")) {
		if h["hook_event_name"] == event {
			out = append(out, h)
		}
	}
	require.NotEmpty(t, out, run+" "+event)
	return out
}

// TestAThoughtHookNamesTheModelThatThoughtItByIdAndParameters: recorded
// (runs/file-tools, a model that reports its thinking), every afterAgentThought
// payload carries the model's id (grok-4.5) and its parameters (effort, speed) beside
// its name; a run's other payloads carry none of the two. The mock hands a
// thought hook the fields the script's thinking block gives, and no payload of
// another event carries them.
// sr:proves hook-common-payload/cursor
func TestAThoughtHookNamesTheModelThatThoughtItByIdAndParameters(t *testing.T) {
	for _, h := range recordedPayloads(t, "file-tools", "afterAgentThought") {
		require.Equal(t, "grok-4.5", h["model_id"])
		require.Equal(t, []any{map[string]any{"id": "effort", "value": "high"}, map[string]any{"id": "speed", "value": "default"}}, h["model_params"])
	}
	for _, h := range readJSONL(t, filepath.Join(newestSample(t, "file-tools"), "payloads.jsonl")) {
		if h["hook_event_name"] != "afterAgentThought" {
			require.NotContains(t, h, "model_id", h["hook_event_name"])
		}
	}

	ws, home, scratch := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".cursor", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks.json"), []byte(`{"version":1,"hooks":{"afterAgentThought":[{"command":".cursor/hooks/log.sh"}],"sessionEnd":[{"command":".cursor/hooks/log.sh"}]}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks", "log.sh"), []byte("#!/bin/sh\ncat >>\"$HOOK_LOG\"\necho >>\"$HOOK_LOG\"\n"), 0o755))
	script := filepath.Join(scratch, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Done.","model":"cursor-grok-4.5-high","model_id":"grok-4.5","model_params":[{"id":"effort","value":"high"},{"id":"speed","value":"default"}]},{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`), 0o755))
	log := filepath.Join(scratch, "log.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	var thoughts int
	for _, h := range readJSONL(t, log) {
		if h["hook_event_name"] == "afterAgentThought" {
			thoughts++
			require.Equal(t, "grok-4.5", h["model_id"])
			require.Len(t, h["model_params"], 2)
		} else {
			require.NotContains(t, h, "model_id", h["hook_event_name"])
		}
	}
	require.Equal(t, 1, thoughts)
}

// TestAShellPostToolUseNamesTheCallAndTimesIt: recorded (runs/tool-failure,
// runs/pretool-refusal), the postToolUse of a Shell call carries the call's
// tool_use_id, a UUID, and a duration in milliseconds above 0 and well under a
// second for a command that returns at once. The mock's does too.
// sr:proves posttool-output/cursor
func TestAShellPostToolUseNamesTheCallAndTimesIt(t *testing.T) {
	uuid := `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	for _, run := range []string{"tool-failure", "pretool-refusal"} {
		var shells int
		for _, h := range recordedPayloads(t, run, "postToolUse") {
			if h["tool_name"] != "Shell" {
				continue
			}
			shells++
			require.Regexp(t, uuid, h["tool_use_id"], run)
			require.Greater(t, h["duration"].(float64), 0.0, run)
			require.Less(t, h["duration"].(float64), 1000.0, run)
		}
		require.Positive(t, shells, run)
		got, _ := replay(t, run)
		var mocks int
		for _, h := range got.raw {
			if h["hook_event_name"] == "postToolUse" && h["tool_name"] == "Shell" {
				mocks++
				require.Regexp(t, uuid, h["tool_use_id"], run+": the mock")
				require.Greater(t, h["duration"].(float64), 0.0, run+": the mock")
				require.Less(t, h["duration"].(float64), 1000.0, run+": the mock")
			}
		}
		require.Equal(t, shells, mocks, run)
	}
}

// TestAReadABeforeReadFileHookBlockedTookSomeTime: recorded
// (runs/before-read-refusal), the postToolUseFailure of a Read a beforeReadFile
// hook blocked carries a duration above 0 (the time the hook took: 0.7 ms to 328
// ms), where a Shell call a preToolUse hook refused took 0 (runs/pretool-refusal).
// The mock's blocked Read has a duration above 0 too.
// sr:proves pretooluse-refusal/cursor
func TestAReadABeforeReadFileHookBlockedTookSomeTime(t *testing.T) {
	for _, h := range recordedPayloads(t, "before-read-refusal", "postToolUseFailure") {
		require.Equal(t, "Read", h["tool_name"])
		require.Greater(t, h["duration"].(float64), 0.0)
	}
	for _, h := range recordedPayloads(t, "pretool-refusal", "postToolUseFailure") {
		require.Equal(t, 0.0, h["duration"], "recorded: a Shell call a preToolUse hook refused took no time")
	}
	got, _ := replay(t, "before-read-refusal")
	var blocked int
	for _, h := range got.raw {
		if h["hook_event_name"] == "postToolUseFailure" && h["tool_name"] == "Read" {
			blocked++
			require.Greater(t, h["duration"].(float64), 0.0, "the mock's blocked Read")
		}
	}
	require.Positive(t, blocked)
}
