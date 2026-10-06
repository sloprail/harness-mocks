package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frames parses stream-json lines, skipping what is not an object.
func frames(text string) (out []map[string]any) {
	for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			out = append(out, f)
		}
	}
	return
}

// taskFrames are the frames of the Task tool call, by subtype.
func taskFrames(fs []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, f := range fs {
		if call, ok := f["tool_call"].(map[string]any); ok && call["taskToolCall"] != nil {
			out[f["subtype"].(string)] = call["taskToolCall"].(map[string]any)
		}
	}
	return out
}

func keys(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// The recorded run runs/foreground-subagent-result: the agent launches one
// foreground sub-agent with the Task tool, asking for the word PINEAPPLE-7.
//
// TestAForegroundSubAgentBlocksItsParentAndItsReportIsTheCallsResult: the
// parent does nothing until the sub-agent has finished (its next message
// follows the call's completed frame, which says how long the sub-agent took),
// and the completed frame holds the sub-agent's final report as the call's
// result, in the fields the recording shows: the report as the last
// conversation step, the sub-agent's id, not in the background, its duration.
// sr:proves foreground-subagent-result/cursor
func TestAForegroundSubAgentBlocksItsParentAndItsReportIsTheCallsResult(t *testing.T) {
	root := filepath.Join("..", "..", "snapshots", "runs", "foreground-subagent-result")
	samples, err := filepath.Glob(filepath.Join(root, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	sort.Strings(samples)
	sample := samples[len(samples)-1]
	rec := jsonLines(t, filepath.Join(sample, "stream.jsonl"))
	want := taskFrames(rec)
	wantArgs := want["started"]["args"].(map[string]any)
	wantSuccess := want["completed"]["result"].(map[string]any)["success"].(map[string]any)
	require.Equal(t, false, wantSuccess["isBackground"])
	var wantHook map[string]any
	for _, p := range jsonLines(t, filepath.Join(sample, "payloads.jsonl")) {
		if p["hook_event_name"] == "preToolUse" {
			wantHook = p["tool_input"].(map[string]any)
		}
	}
	require.NotNil(t, wantHook)
	require.Equal(t, false, wantHook["run_in_background"])

	// the recorded call, replayed: its description and prompt, no subagent type
	ws := t.TempDir()
	cp := func(name, to string, mode os.FileMode) {
		b, err := os.ReadFile(filepath.Join(root, "setup", name))
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(ws, to)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(ws, to), b, mode))
	}
	cp("hooks.json", ".cursor/hooks.json", 0o644)
	cp("hook.sh", ".cursor/hooks/hook.sh", 0o755)
	sub := filepath.Join(ws, "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(`#!/bin/sh
sleep 0.4
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PINEAPPLE-7"}]}}'
`), 0o755))
	input, err := json.Marshal(map[string]any{"description": wantArgs["description"], "prompt": wantArgs["prompt"], "run_in_background": false, "script": sub})
	require.NoError(t, err)
	task := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_1","name":"Task","input":` + string(input) + `}]}}`
	main := filepath.Join(ws, "main.sh")
	require.NoError(t, os.WriteFile(main, []byte(`#!/bin/sh
if grep -q tool_use "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PINEAPPLE-7"}]}}' '`+done+`'
else
  printf '%s\n' '`+task+`'
fi
`), 0o755))
	log := filepath.Join(ws, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", main, "launch one")
	home := t.TempDir()
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log}
	out, err := cmd.Output()
	require.NoError(t, err, "%s", out)
	got := frames(string(out))
	gotTask := taskFrames(got)
	require.Contains(t, gotTask, "started")
	require.Contains(t, gotTask, "completed")

	// the preToolUse hook for the call: the same tool_input as recorded
	var gotHook map[string]any
	for _, p := range jsonLines(t, log) {
		if p["hook_event_name"] == "preToolUse" {
			gotHook = p["tool_input"].(map[string]any)
		}
	}
	require.NotNil(t, gotHook, "the preToolUse hook ran for the Task call")
	assert.Equal(t, wantHook, gotHook)

	// the hooks the run's hooks.json registers for the sub-agent's end and the
	// call's end never fire: only the call's preToolUse does, recorded and mock
	fired := func(ps []map[string]any) (events []string, sessions map[string]bool) {
		sessions = map[string]bool{}
		for _, p := range ps {
			if e, ok := p["hook_event_name"].(string); ok {
				events = append(events, e)
				if s, ok := p["session_id"].(string); ok {
					sessions[s] = true
				}
			}
		}
		return
	}
	recEvents, recSessions := fired(jsonLines(t, filepath.Join(sample, "payloads.jsonl")))
	gotEvents, _ := fired(jsonLines(t, log))
	for name, events := range map[string][]string{"recorded": recEvents, "mock": gotEvents} {
		for _, e := range []string{"postToolUse", "postToolUseFailure", "subagentStart", "subagentStop"} {
			assert.NotContains(t, events, e, name+": "+e+" does not fire for a foreground Task call")
		}
		assert.Contains(t, events, "preToolUse", name)
	}
	assert.Greater(t, len(recSessions), 1, "recorded: the sub-agent's own events carry a session id of their own")

	// the call's args: the values the recording shows for what the mock carries
	// (the agent id is the call's own, a fresh one)
	gotArgs := gotTask["started"]["args"].(map[string]any)
	for _, k := range []string{"description", "prompt", "subagentType", "model"} {
		assert.Equal(t, wantArgs[k], gotArgs[k], k)
	}
	assert.NotEmpty(t, gotArgs["agentId"])
	assert.Equal(t, gotArgs, gotTask["completed"]["args"], "the completed frame repeats the call's args")

	gotSuccess := gotTask["completed"]["result"].(map[string]any)["success"].(map[string]any)
	assert.Equal(t, keys(wantSuccess), keys(gotSuccess))
	assert.Equal(t, false, gotSuccess["isBackground"])
	assert.Equal(t, wantSuccess["conversationSteps"], gotSuccess["conversationSteps"], "the report is the sub-agent's last words")
	assert.Contains(t, gotTask["completed"]["result"].(map[string]any)["success"].(map[string]any)["conversationSteps"].([]any)[0].(map[string]any)["assistantMessage"].(map[string]any)["text"], "PINEAPPLE-7")
	assert.NotEmpty(t, gotSuccess["agentId"])
	// the result's agent id is the sub-agent's own conversation id, not the id the call's args carry
	// (recorded: the sub-agent's transcript is named by it, and it differs from args.agentId)
	recTranscripts, err := filepath.Glob(filepath.Join(sample, "transcript", "*"))
	require.NoError(t, err)
	var recIDs []string
	for _, d := range recTranscripts {
		recIDs = append(recIDs, filepath.Base(d))
	}
	assert.Contains(t, recIDs, wantSuccess["agentId"], "recorded: the result's agentId names the sub-agent's transcript")
	assert.NotEqual(t, wantArgs["agentId"], wantSuccess["agentId"], "recorded: it differs from the args' agentId")
	subTranscripts, err := filepath.Glob(filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts", "*"))
	require.NoError(t, err)
	var gotIDs []string
	for _, d := range subTranscripts {
		gotIDs = append(gotIDs, filepath.Base(d))
	}
	assert.Contains(t, gotIDs, gotSuccess["agentId"], "the mock's result agentId names the sub-agent's transcript")
	assert.NotEqual(t, gotArgs["agentId"], gotSuccess["agentId"], "and differs from the args' agentId")
	assert.Equal(t, wantSuccess["backgroundReason"], gotSuccess["backgroundReason"])
	ms, err := strconv.Atoi(gotSuccess["durationMs"].(string))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, ms, 400, "the call lasted as long as the sub-agent ran")

	// the parent blocked: the call ended before its next message, and the run's
	// result carries the report it then gave, as recorded
	order := func(fs []map[string]any) (o []string) {
		for _, f := range fs {
			switch f["type"] {
			case "tool_call":
				o = append(o, "tool_call/"+f["subtype"].(string))
			case "assistant", "result":
				o = append(o, f["type"].(string))
			}
		}
		return o
	}
	wantOrder := []string{"assistant", "tool_call/started", "tool_call/completed", "assistant", "result"}
	require.Equal(t, wantOrder, order(rec), "the recording")
	assert.Equal(t, wantOrder[1:], order(got))
	last := got[len(got)-1]
	assert.Equal(t, "result", last["type"])
	assert.Contains(t, last["result"], "PINEAPPLE-7")
	assert.Contains(t, rec[len(rec)-1]["result"], "PINEAPPLE-7")
}
