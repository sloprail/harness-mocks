package e2e

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/task-stream-frames: one prompt that spawns a sub-agent
// to reply PONG, runs `sleep 5; echo BGDONE`, and waits for the sub-agent.

// collabShape is the stream's collab_tool_call items without the thread ids,
// which differ between runs: the event, tool and status of each, how many
// sub-agents it names, and the status and message of each state it carries.
func collabShape(events []map[string]any) (out []string) {
	for _, e := range events {
		item, _ := e["item"].(map[string]any)
		if item["type"] != "collab_tool_call" {
			continue
		}
		var states []string
		for _, s := range item["agents_states"].(map[string]any) {
			st := s.(map[string]any)
			states = append(states, fmt.Sprintf("%v=%v", st["status"], st["message"]))
		}
		sort.Strings(states)
		out = append(out, fmt.Sprintf("%v %v %v receivers=%d prompt=%v states=%v", e["type"], item["tool"], item["status"],
			len(item["receiver_thread_ids"].([]any)), item["prompt"], states))
	}
	return
}

// A sub-agent is announced when it is spawned: the spawn call's item starts,
// then completes naming the sub-agent's thread, which is still pending. Its
// status and its end are told in the item of the wait on it, as its state,
// "completed" with its answer; nothing else on the stream is a frame of the
// task, and a shell command is no task of its own, only a command_execution
// item (runs/task-stream-frames).
// sr:proves task-stream-frames/codex
func TestASubAgentIsAnnouncedBySpawnAndItsEndIsToldByTheWait(t *testing.T) {
	rec := loadRecording(t, "task-stream-frames")
	want := collabShape(jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl"))))
	require.Len(t, want, 4)

	script := `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
case $n in
0) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"spawn_agent","input":{"message":"Reply with the single word PONG.","script":"sub.sh"}}]}}';;
1) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c1","name":"Bash","input":{"command":"sh -c '"'"'echo BGDONE'"'"'"}}]}}';;
*) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}';;
esac
`
	sub := `#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PONG"}]}}' '{"type":"result","subtype":"success","result":"PONG"}'
`
	got := execMock(t, scenario{Script: script, Files: map[string]string{"sub.sh": sub}, Prompt: "go"})
	require.Equal(t, 0, got.Code, got.Stderr)

	// the mock's items are the recording's, apart from the order: the mock
	// runs the calls one after the other, where the real agent made the spawn
	// and the command together
	assert.Equal(t, want, collabShape(got.stream()))

	// the answer is what the agent was told by the wait
	rollout := got.rollout(t)
	assert.Contains(t, rollout, `\"completed\":\"PONG\"`)

	// a sub-agent's thread is named by the spawn, and the wait names the same
	var spawned, waited []any
	for _, e := range got.stream() {
		item, _ := e["item"].(map[string]any)
		if e["type"] == "item.completed" && item["type"] == "collab_tool_call" {
			if item["tool"] == "spawn_agent" {
				spawned = item["receiver_thread_ids"].([]any)
			} else {
				waited = item["receiver_thread_ids"].([]any)
			}
		}
	}
	require.Len(t, spawned, 1)
	assert.Equal(t, spawned, waited)
}
