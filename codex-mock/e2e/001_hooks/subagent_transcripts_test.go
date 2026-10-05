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

// The recorded run runs/subagent-transcripts-v2 (the default multi-agent mode): the agent spawns one
// sub-agent (task "ping", which answers PONG) and waits for it. The model's
// spawn_agent call is replayed by a script, the sub-agent's answer by another.
const (
	pingSpawn = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_spawn","name":"spawn_agent","input":{"task_name":"ping","message":"Reply with PONG","script":"pong.sh"}}]}}'
  exit 0
fi
if [ "$n" = 1 ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_wait","name":"wait_agent","input":{"targets":["%s"],"timeout_ms":60000}}]}}\n' "$(jq -r 'select(.payload.type=="function_call_output")|.payload.output|try (fromjson|.agent_id) catch empty|select(.!=null)' "$A10N_MOCK_SESSION_FILE" | head -1)"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`
	pongScript = `#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PONG"}]}}' '{"type":"result","subtype":"success","result":"PONG"}'
`
)

// rollouts are the session files under dir, by path, parsed.
func rolloutFiles(t *testing.T, dir string) map[string][]map[string]any {
	t.Helper()
	out := map[string][]map[string]any{}
	require.NoError(t, filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".jsonl") {
			out[p] = jsonLines(readFile(t, p))
		}
		return nil
	}))
	return out
}

// metaOf is the first record's payload of a rollout.
func metaOf(rec []map[string]any) map[string]any {
	m, _ := rec[0]["payload"].(map[string]any)
	return m
}

// splitRollouts tells the session's rollout from the sub-agent's: the one
// whose meta names a parent thread is the sub-agent's.
func splitRollouts(t *testing.T, files map[string][]map[string]any) (parentPath, childPath string) {
	t.Helper()
	require.Len(t, files, 2, "the session's rollout and the sub-agent's")
	for p, rec := range files {
		if _, ok := metaOf(rec)["parent_thread_id"]; ok {
			childPath = p
		} else {
			parentPath = p
		}
	}
	require.NotEmpty(t, parentPath)
	require.NotEmpty(t, childPath)
	return
}

// saidBy are the texts of the assistant messages of a rollout.
func saidBy(rec []map[string]any) (out []string) {
	for _, r := range rec {
		p, _ := r["payload"].(map[string]any)
		if p["type"] == "message" && p["role"] == "assistant" {
			b, _ := json.Marshal(p["content"])
			out = append(out, string(b))
		}
	}
	return
}

// A sub-agent's records go to a rollout of its own, beside the session's (the
// same directory, named the same way) and not into it. Its meta record names
// what spawned it (the parent thread) and how deep it is; what the sub-agent said is in its file, not the
// session's. There is no sidecar file (runs/subagent-transcripts-v2).
// sr:proves subagent-transcripts/codex
func TestSubAgentRecordsGoToARolloutOfItsOwn(t *testing.T) {
	rec := loadRecording(t, "subagent-transcripts-v2")
	want := rolloutFiles(t, filepath.Join(rec.sample, "transcript"))
	wantParent, wantChild := splitRollouts(t, want)

	got := execMock(t, scenario{Script: pingSpawn, Files: map[string]string{"pong.sh": pongScript},
		Prompt: strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt")))})
	require.Equal(t, 0, got.Code, got.Stderr)
	have := rolloutFiles(t, got.Home)
	parentPath, childPath := splitRollouts(t, have)

	for name, p := range map[string]string{"session": parentPath, "sub-agent": childPath,
		"recorded session": wantParent, "recorded sub-agent": wantChild} {
		assert.Regexp(t, `^rollout-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-.+\.jsonl$`, filepath.Base(p), name)
	}
	assert.Equal(t, filepath.Dir(wantParent), filepath.Dir(wantChild), "recording: beside the session's")
	assert.Equal(t, filepath.Dir(parentPath), filepath.Dir(childPath), "mock: beside the session's")
	entries, err := os.ReadDir(filepath.Dir(childPath))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no sidecar file")

	wantMeta, gotMeta := metaOf(want[wantChild]), metaOf(have[childPath])
	spawn := func(m map[string]any) map[string]any {
		return m["source"].(map[string]any)["subagent"].(map[string]any)["thread_spawn"].(map[string]any)
	}
	sessionID := metaOf(have[parentPath])["id"]
	assert.Equal(t, sessionID, gotMeta["parent_thread_id"], "what spawned it")
	assert.Equal(t, sessionID, gotMeta["session_id"], "the session it belongs to")
	assert.Equal(t, sessionID, spawn(gotMeta)["parent_thread_id"])
	assert.Equal(t, wantMeta["thread_source"], gotMeta["thread_source"])
	assert.Nil(t, wantMeta["agent_path"], "recording: no path in the tree of agents")
	assert.Nil(t, gotMeta["agent_path"], "mock: none either")
	assert.Equal(t, spawn(wantMeta)["depth"], spawn(gotMeta)["depth"], "its depth")
	assert.EqualValues(t, 1, spawn(gotMeta)["depth"])
	assert.NotEqual(t, sessionID, gotMeta["id"], "a thread of its own")
	assert.Equal(t, "exec", metaOf(have[parentPath])["source"], "the session is not a sub-agent")

	// no sidecar and no spawning call: the meta record names a role that is
	// null and no call id, in the recording and in the mock
	for name, m := range map[string]map[string]any{"recording": wantMeta, "mock": gotMeta} {
		assert.Contains(t, spawn(m), "agent_role", name)
		assert.Nil(t, spawn(m)["agent_role"], "%s: no agent type", name)
		assert.NotContains(t, toJSON(m), "call_id", "%s: no spawning call", name)
	}
	// the answer reaches the session as the output of the spawn call
	assert.Contains(t, toJSON(have[parentPath]), `function_call_output`)
	assert.Contains(t, toJSON(have[parentPath]), `PONG`, "the mock tells the session the answer through the call's output")
	assert.NotContains(t, toJSON(saidBy(have[parentPath])), "PONG")

	// what the sub-agent said is in its file, and only there
	assert.Len(t, saidBy(want[wantChild]), 1)
	assert.Len(t, saidBy(have[childPath]), 1)
	assert.Contains(t, saidBy(have[childPath])[0], "PONG")
	assert.Contains(t, saidBy(want[wantChild])[0], "PONG")
	for name, rs := range map[string][]map[string]any{"recording": want[wantParent], "mock": have[parentPath]} {
		assert.Len(t, saidBy(rs), 1, name)
		assert.NotContains(t, saidBy(rs)[0], "PONG", "%s: the session's file does not hold the sub-agent's words", name)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// notice is a <subagent_notification> message of a rollout: the user's message the
// harness adds when a sub-agent has ended, with where it sits among the records.
type notice struct {
	Agent, Completed string
	AfterOutput      bool // the record before it is a tool output
	BeforeAnswer     bool // the record after it is the assistant's message
}

func noticesOf(t *testing.T, rec []map[string]any) (out []notice) {
	t.Helper()
	var items []map[string]any
	for _, r := range rec {
		if p, ok := r["payload"].(map[string]any); ok && r["type"] == "response_item" && p["type"] != "reasoning" {
			items = append(items, p)
		}
	}
	for i, p := range items {
		if p["type"] != "message" || p["role"] != "user" {
			continue
		}
		c, _ := p["content"].([]any)[0].(map[string]any)["text"].(string)
		if !strings.HasPrefix(c, "<subagent_notification>") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(c, "<subagent_notification>\n"), "\n</subagent_notification>")
		var n struct {
			AgentPath string            `json:"agent_path"`
			Status    map[string]string `json:"status"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &n))
		out = append(out, notice{Agent: n.AgentPath, Completed: n.Status["completed"],
			AfterOutput:  i > 0 && strings.HasSuffix(toJSON(items[i-1]["type"]), `_output"`),
			BeforeAnswer: i+1 < len(items) && items[i+1]["role"] == "assistant"})
	}
	return out
}

// The harness tells the session when a sub-agent it started has ended, as a
// <subagent_notification> message of the user's naming the sub-agent and its
// final answer, after the tool output the agent was given and before its next
// answer, once, though the agent had waited for the sub-agent (recorded:
// runs/subagent-transcripts-v2).
// sr:proves subagent-transcripts/codex
func TestTheSessionIsToldWhenASubAgentEnds(t *testing.T) {
	rec := loadRecording(t, "subagent-transcripts-v2")
	want := rolloutFiles(t, filepath.Join(rec.sample, "transcript"))
	wantParent, wantChild := splitRollouts(t, want)

	got := execMock(t, scenario{Script: pingSpawn, Files: map[string]string{"pong.sh": pongScript},
		Prompt: strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt")))})
	require.Equal(t, 0, got.Code, got.Stderr)
	have := rolloutFiles(t, got.Home)
	parentPath, childPath := splitRollouts(t, have)

	for name, c := range map[string]struct {
		notices []notice
		child   string
	}{
		"recording": {noticesOf(t, want[wantParent]), metaOf(want[wantChild])["id"].(string)},
		"mock":      {noticesOf(t, have[parentPath]), metaOf(have[childPath])["id"].(string)},
	} {
		require.Len(t, c.notices, 1, name)
		assert.Equal(t, notice{Agent: c.child, Completed: "PONG", AfterOutput: true, BeforeAnswer: true}, c.notices[0], name)
	}
}
