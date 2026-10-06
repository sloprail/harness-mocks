package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collabTools are the multi-agent tools a stream shows completed, in order.
func collabTools(r result) (tools []string) {
	for _, e := range r.stream() {
		if item, _ := e["item"].(map[string]any); item["type"] == "collab_tool_call" && e["type"] == "item.completed" {
			tools = append(tools, item["tool"].(string))
		}
	}
	return
}

// A run does not wait for a sub-agent nobody waits for: the main thread, having
// spawned one and not waited for it, ends the run, whose stream has the spawn and
// no wait (runs/nested-subagents-nowait: the sub-agent it left running was
// aborted). The mock does the same: a spawn is answered at once, and the
// stream has a wait only when the agent calls wait_agent.
// sr:proves nested-subagents/codex
func TestTheRunEndsWithoutWaitingForASubAgentNobodyWaitedFor(t *testing.T) {
	rec := nestedRecording(t, "nested-subagents-nowait")
	assert.Equal(t, []string{"spawn_agent"}, collabTools(result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}),
		"the recording has no wait")

	leaf := filepath.Join(t.TempDir(), "leaf.sh")
	require.NoError(t, os.WriteFile(leaf, []byte(chainScript("")), 0o755))
	waited := execMock(t, scenario{Script: chainScript(leaf), Prompt: "go"})
	require.Equal(t, 0, waited.Code, waited.Stderr)
	assert.Equal(t, []string{"spawn_agent", "wait"}, collabTools(waited), "a spawn followed by a wait_agent call")

	// the recorded sub-agent was slow (sleep 25): the run ended long before it
	slow := filepath.Join(t.TempDir(), "slow.sh")
	require.NoError(t, os.WriteFile(slow, []byte(strings.Replace(chainScript(""), "echo LEAF", "sleep 8; echo LEAF", 1)), 0o755))
	main := `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_0","name":"spawn_agent","input":{"message":"go","script":"` + slow + `"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`
	started := time.Now()
	got := execMock(t, scenario{Script: main, Prompt: "go"})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Less(t, time.Since(started), 6*time.Second, "the run ended without waiting for the sub-agent")
	assert.Equal(t, []string{"spawn_agent"}, collabTools(got), "a spawn nobody waits for is not waited for")
	ths, _ := threads(t, filepath.Join(got.Home, "sessions"))
	for _, th := range ths {
		assert.NotContains(t, th.Told, "LEAF\n", "the slow sub-agent never finished what it was doing")
	}
}
