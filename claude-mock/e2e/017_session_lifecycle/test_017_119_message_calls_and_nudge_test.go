package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func frameKinds(frames []map[string]any) (out []string) {
	for _, f := range frames {
		switch {
		case f["type"] == "assistant":
			c := f["message"].(map[string]any)["content"].([]any)
			if b := c[0].(map[string]any); b["type"] == "tool_use" {
				out = append(out, "call:"+b["name"].(string))
			}
		case f["type"] == "user" && f["isSynthetic"] == true:
			out = append(out, "nudge")
		case f["type"] == "user":
			if c, ok := f["message"].(map[string]any)["content"].([]any); ok && c[0].(map[string]any)["type"] == "tool_result" {
				out = append(out, "result")
			}
		}
	}
	return out
}

// TestT017_119_SeveralCallsInOneMessage: the model's message may hold several tool calls (the
// script marks each but the last with "more"): their frames come first, then each call is carried out
// and answered in turn, every call counts as a turn, and the PreToolUse hooks of all come before the
// first call's PostToolUse (recording bgagent-concurrent-limit, sample 150658).
// sr:proves background-agent/claude
func TestT017_119_SeveralCallsInOneMessage(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgagent-concurrent-limit/samples/20261006-150658/stream.jsonl"))
	require.NoError(t, err)
	rec := frameKinds(streamFrames(t, string(data)))
	require.Equal(t, []string{"call:Agent", "call:Agent", "result", "result"}, rec[:4], "recorded")

	dir := t.TempDir()
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PreToolUse": h, "PostToolUse": h})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
n=$(grep -c '"type":"tool_result"' "$A10N_MOCK_SESSION_FILE")
case "$n" in
0) echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"Bash","input":{"command":"echo A"},"more":true}]}}'
   echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"c2","name":"Bash","input":{"command":"echo B"}}]}}' ;;
*) echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"DONE"}]}}'
   echo '{"type":"result","subtype":"success","result":"DONE"}' ;;
esac
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "mc-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "go")
	require.Equal(t, 0, code, out)
	frames := streamFrames(t, out)
	assert.Equal(t, []string{"call:Bash", "call:Bash", "result", "result"}, frameKinds(frames)[:4])
	assert.NotContains(t, out, `"more"`, "the marker is no part of what the model sent")
	var events []string
	for _, p := range payloads(t, log) {
		events = append(events, p["hook_event_name"].(string)+":"+p["tool_use_id"].(string)[:2])
	}
	assert.Equal(t, []string{"PreToolUse:c1", "PreToolUse:c2", "PostToolUse:c1", "PostToolUse:c2"}, events)
	result := frames[len(frames)-1]
	assert.EqualValues(t, 3, result["num_turns"], "two calls and the answer")
}

// TestT017_120_AResponseWithNoVisibleOutputIsNudged: a response of only thinking is answered by the
// harness with a synthetic user message, and the model goes on: no result between, and no Stop hook
// for the empty response (recording bgagent-concurrent-limit, sample 150755).
// sr:proves task-notifications/claude
func TestT017_120_AResponseWithNoVisibleOutputIsNudged(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgagent-concurrent-limit/samples/20261006-150755/stream.jsonl"))
	require.NoError(t, err)
	require.Contains(t, frameKinds(streamFrames(t, string(data))), "nudge", "recorded")

	dir := t.TempDir()
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"Stop": h})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
if ! grep -q 'no visible output' "$A10N_MOCK_SESSION_FILE"; then
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":""}]}}'
echo '{"type":"result","subtype":"success","result":""}'
exit 0
fi
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"NOW VISIBLE"}]}}'
echo '{"type":"result","subtype":"success","result":"NOW VISIBLE"}'
`, 0o755)
	cfg := filepath.Join(dir, "config")
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "nv-1", "--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "go")
	require.Equal(t, 0, code, out)
	frames := streamFrames(t, out)
	var results int
	for _, f := range frames {
		if f["type"] == "result" {
			results++
		}
	}
	assert.Equal(t, 1, results, "one result: the empty response ended nothing")
	assert.Contains(t, frameKinds(frames), "nudge")
	assert.Len(t, payloads(t, log), 1, "one Stop, for the answer that was visible")
	assert.Contains(t, readString(t, transcriptPath(t, cfg, dir, "nv-1")), `"turnCompanion":true`)
}

// TestT017_121_ASubagentStartedAfterTheLaunchingCallsPostToolUse: the order of a background
// sub-agent's SubagentStart and its launching call's PostToolUse is the harness's race; the mock
// follows the scenario's mock-only "mock_start_after_post" (the replay reads it from each sample),
// and the SubagentStart then comes before the next call of the message is taken.
// sr:proves background-agent/claude
func TestT017_121_ASubagentStartedAfterTheLaunchingCallsPostToolUse(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PostToolUse": h, "SubagentStart": h})
	sub := script(t, dir, "sub")
	order := func(late bool) []string {
		os.Remove(log)
		sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
n=$(grep -c '"type":"tool_result"' "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"prompt":"p","description":"d","run_in_background":true,"script":"`+sub+`","mock_start_after_post":`+map[bool]string{true: "true", false: "false"}[late]+`}}]}}'
else
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"DONE"}]}}'
echo '{"type":"result","subtype":"success","result":"DONE"}'
fi
`, 0o755)
		out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "sa-"+map[bool]string{true: "l", false: "e"}[late], "--project-dir", dir,
			"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "go")
		require.Equal(t, 0, code, out)
		var ev []string
		for _, p := range payloads(t, log) {
			ev = append(ev, p["hook_event_name"].(string))
		}
		return ev
	}
	assert.Equal(t, []string{"SubagentStart", "PostToolUse"}, order(false), "at the launch, ahead of the call's PostToolUse")
	assert.Equal(t, []string{"PostToolUse", "SubagentStart"}, order(true), "after it, as the sample shows")
}
