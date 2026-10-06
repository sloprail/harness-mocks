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

// The recorded run runs/background-agent (captured with the real codex 0.159.3, retry 1): the agent spawns a sub-agent, then
// runs a command of its own that outlasts the sub-agent's.
const bgSpawnScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
emit() { printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_%s","name":"%s","input":%s}]}}\n' "$n" "$1" "$2"; }
end() { printf '{"type":"assistant","message":{"content":[{"type":"text","text":"%s"}]}}\n{"type":"result","subtype":"success","result":"%s"}\n' "$1" "$1"; }
case "$n" in
0) emit spawn_agent "$(jq -nc --arg m "$SPAWN_MSG" '{message: $m, script: "sub.sh"}')" ;;
1) emit Bash "$(jq -nc --arg c "$PARENT_CMD" '{command: $c}')" ;;
*) end LAUNCHED ;;
esac
`

// bgSubScript drives the sub-agent: its one command, then its reply.
const bgSubScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_s","name":"Bash","input":%s}]}}\n' "$(jq -nc --arg c "$SUB_CMD" '{command: $c}')"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"SUBREPLY"}]}}' '{"type":"result","subtype":"success","result":"SUBREPLY"}'
`

// hookFacts is what the hooks saw, by event, tool and who it was for, without
// the ids a run makes up.
func hookFacts(lines []map[string]any) []string {
	var out []string
	for _, l := range lines {
		input := l["tool_input"]
		if m, ok := input.(map[string]any); ok && l["tool_name"] == "spawn_agent" {
			input = map[string]any{"message": m["message"]} // the mock's own parameters are not Codex's
		}
		in, _ := json.Marshal(input)
		_, sub := l["agent_id"]
		out = append(out, strings.Join([]string{l["hook_event_name"].(string), str(l["tool_name"]), string(in), map[bool]string{true: "sub-agent", false: "main"}[sub]}, " "))
	}
	return out
}

func str(v any) string { s, _ := v.(string); return s }

// shape is a stream's sub-agent and command items without the ids.
func shape(stream []map[string]any) (out []string) {
	for _, e := range stream {
		item, _ := e["item"].(map[string]any)
		if item == nil || (item["type"] != "collab_tool_call" && item["type"] != "command_execution") {
			continue
		}
		states, _ := item["agents_states"].(map[string]any)
		var st []string
		for _, s := range states {
			st = append(st, str(s.(map[string]any)["status"]))
		}
		recv, _ := item["receiver_thread_ids"].([]any)
		out = append(out, strings.Join([]string{str(e["type"]), str(item["type"]), str(item["tool"]), str(item["status"]),
			strings.Join(st, ","), string(rune('0' + len(recv)))}, " "))
	}
	return
}

// A sub-agent spawned in the background: the agent is given a receipt at once
// (the sub-agent's id, in the call's result and in the event stream, where its
// thread is started but not yet running), the turn goes on, and the sub-agent
// runs concurrently with it: its own command's hooks, which name it, fire
// while the agent's own command is still running. The main thread's hooks
// name no agent (runs/background-agent).
// sr:proves background-agent/codex
// sr:proves hook-common-payload/codex
func TestSubAgentSpawnedGivesReceiptAndRunsConcurrently(t *testing.T) {
	// (loadRecording takes every tool call for a command, which a spawn is not)
	samples, err := filepath.Glob(filepath.Join(runsDir, "background-agent", "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	rec := recording{setup: filepath.Join(runsDir, "background-agent", "setup"), sample: samples[len(samples)-1]}
	var spawn, subCmd, parentCmd string
	for _, p := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		in, _ := p["tool_input"].(map[string]any)
		if p["hook_event_name"] != "PreToolUse" {
			continue
		}
		switch {
		case p["tool_name"] == "spawn_agent":
			spawn = str(in["message"])
		case p["agent_id"] != nil:
			subCmd = str(in["command"])
		default:
			parentCmd = str(in["command"])
		}
	}
	require.NotEmpty(t, spawn)
	require.NotEmpty(t, subCmd)
	require.NotEmpty(t, parentCmd)
	parentCmd = strings.Replace(parentCmd, "sleep 10", "sleep 2", 1) // the sub-agent's command is done well within it

	// the recorded hooks, and the same one for SessionEnd: the session ends
	// once, for the main thread, and not for the sub-agent
	var conf map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(readFile(t, filepath.Join(rec.setup, "hooks.json"))), &conf))
	conf["hooks"]["SessionEnd"] = conf["hooks"]["Stop"]
	confJSON, err := json.Marshal(conf)
	require.NoError(t, err)

	got := execMock(t, scenario{
		HooksJSON: string(confJSON),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh")), "sub.sh": bgSubScript},
		Script:    bgSpawnScript,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       []string{"SPAWN_MSG=" + spawn, "SUB_CMD=" + subCmd, "PARENT_CMD=" + parentCmd},
	})
	require.Equal(t, 0, got.Code, got.Stderr)

	// the stream shows the spawn begin, then complete with the sub-agent's thread
	// pending, before the agent's own command, as recorded
	wantStream := jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl")))
	assert.Equal(t, shape(wantStream), shape(got.stream()))
	var agent string
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); item["tool"] == "spawn_agent" && e["type"] == "item.completed" {
			agent = str(item["receiver_thread_ids"].([]any)[0])
		}
	}
	require.NotEmpty(t, agent)

	// the receipt, told to the agent and to the hook, names the sub-agent
	var told struct {
		AgentID  string `json:"agent_id"`
		Nickname string `json:"nickname"`
	}
	var rollouts string // the agent's and the sub-agent's, each its own file
	require.NoError(t, filepath.Walk(filepath.Join(got.Home, "sessions"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rollouts += readFile(t, p)
		}
		return nil
	}))
	assert.Contains(t, rollouts, `\"agent_id\":\"`+agent+`\"`)
	var post map[string]any
	for _, l := range got.hookLog() {
		if l["hook_event_name"] == "PostToolUse" && l["tool_name"] == "spawn_agent" {
			post = l
		}
	}
	require.NotNil(t, post)
	require.NoError(t, json.Unmarshal([]byte(post["tool_response"].(string)), &told))
	assert.Equal(t, agent, told.AgentID)
	assert.NotEmpty(t, told.Nickname)

	// the same hooks fired, in the order the recording shows: the spawn's, the
	// agent's command begun, the sub-agent's command run to its end, and only
	// then the agent's command ended
	var log []map[string]any
	ended := 0
	for _, l := range got.hookLog() {
		if l["hook_event_name"] == "SessionEnd" {
			ended++
			assert.NotContains(t, l, "agent_id", "the session ends for the main thread only")
			continue
		}
		log = append(log, l)
	}
	assert.Equal(t, 1, ended)
	facts := hookFacts(log)
	var want []string
	for _, f := range hookFacts(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))) {
		want = append(want, strings.Replace(f, "sleep 10", "sleep 2", 1))
	}
	assert.ElementsMatch(t, want, facts)
	at := func(fact string) int {
		for i, f := range facts {
			if strings.HasPrefix(f, fact) {
				return i
			}
		}
		return -1
	}
	postSpawn := at("PostToolUse spawn_agent")
	subPre, subPost := at("PreToolUse Bash {\"command\":\""+subCmd), at("PostToolUse Bash {\"command\":\""+subCmd)
	parentPre, parentPost := at("PreToolUse Bash {\"command\":\""+parentCmd), at("PostToolUse Bash {\"command\":\""+parentCmd)
	require.NotEqual(t, -1, subPost)
	assert.Less(t, postSpawn, subPre, "the sub-agent runs once the spawn is answered")
	assert.Less(t, postSpawn, parentPre)
	assert.Less(t, subPost, parentPost, "the sub-agent's command ended while the agent's was still running")
	assert.Less(t, parentPre, parentPost)
	recorded := jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	for _, p := range recorded { // as recorded
		if p["agent_id"] != nil {
			assert.NotEqual(t, recorded[0]["transcript_path"], p["transcript_path"], "recorded: the sub-agent's transcript is its own")
			assert.Contains(t, p["transcript_path"], str(p["agent_id"])+".jsonl")
			assert.Equal(t, "<RUN>", p["cwd"], "recorded: the sub-agent's hooks run in the run's directory")
		}
	}
	// every hook, the sub-agent's too, carries the session's own id (the doc: a
	// sub-agent's hooks use the parent session id); the sub-agent's is told apart
	// by agent_id, which the main thread's hooks do not have
	for _, l := range log {
		assert.Equal(t, log[0]["session_id"], l["session_id"], l["hook_event_name"])
		if l["tool_name"] == "Bash" && strings.Contains(str(l["tool_input"].(map[string]any)["command"]), subCmd) {
			assert.Equal(t, agent, l["agent_id"])
			assert.Equal(t, "default", l["agent_type"])
			// its transcript is its own rollout, named by its id, not the session's; its directory the run's
			assert.NotEqual(t, log[0]["transcript_path"], l["transcript_path"], "the sub-agent's transcript is its own")
			assert.Contains(t, l["transcript_path"], agent+".jsonl")
			assert.Equal(t, got.Repo, evalDir(t, str(l["cwd"])))
			if l["hook_event_name"] == "PostToolUse" {
				assert.Equal(t, "SUBDONE\n", l["tool_response"])
			}
		} else {
			assert.NotContains(t, l, "agent_id", "the main thread's hooks name no agent")
			assert.NotContains(t, l, "agent_type", "nor a type")
		}
	}
}

// evalDir is dir with symlinks resolved, as the mock reports its working directory.
func evalDir(t *testing.T, dir string) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return d
}
