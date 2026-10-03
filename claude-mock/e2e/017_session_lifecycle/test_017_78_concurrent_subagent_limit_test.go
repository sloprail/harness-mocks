package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// concurrentRefusal is what the real claude answered the Agent call refused
// for the concurrent limit with (recorded: bgagent-concurrent-limit).
const concurrentRefusal = "Concurrent subagent limit reached. You can run 1 subagents at once. Do not retry. If the user wants more concurrent subagents, ask them to increase CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS."

// fromFirst is n labels from the first one named first.
func fromFirst(labels []string, first string, n int) []string {
	for i, l := range labels {
		if l == first {
			return labels[i:min(i+n, len(labels))]
		}
	}
	return nil
}

// limitRun launches the Agent calls (each a tool_use line of the orchestrator)
// with the concurrent limit at 1, and returns the hook payloads and the
// transcript's records.
func limitRun(t *testing.T, session string, calls func(dir string) []string) ([]map[string]any, []rec) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, allHooks(payloadLogger(t, dir, "log.sh", log, "")))
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS=1"}, "--script", script(t, dir, "orch", calls(dir)...),
		"--session-id", session, "--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	return payloads(t, log), readRecs(t, transcriptPath(t, cfg, dir, session))
}

// slowReply is a sub-agent that is still running a while after it starts.
func slowReply(t *testing.T, dir, text string) string {
	t.Helper()
	return write(t, filepath.Join(dir, "slow.sh"), "#!/bin/sh\nsleep 3\nexec sh "+replyScript(t, dir, "slowReply", text)+"\n", 0o755)
}

// TestT017_78_ConcurrentSubagentLimit: with CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS
// at 1 and a sub-agent running, spawning another fails (sub-agents doc,
// "Concurrent subagent limit"): replays the recorded bgagent-concurrent-limit
// run, where the second launch gets the tool error telling Claude not to retry
// (is_error, toolUseResult "Error: <message>"), PreToolUse fires and
// PostToolUseFailure answers it with that message and is_interrupt false, and no
// sub-agent starts for it.
// sr:proves background-agent/claude
func TestT017_78_ConcurrentSubagentLimit(t *testing.T) {
	got, recs := limitRun(t, "lim-1", func(dir string) []string {
		return []string{
			toolUse("ag1", "Agent", `{"prompt":"one","description":"one","run_in_background":true,"script":"`+slowReply(t, dir, "ONE")+`"}`),
			toolUse("ag2", "Agent", `{"prompt":"two","description":"two","run_in_background":true,"script":"`+replyScript(t, dir, "two", "TWO")+`"}`),
		}
	})
	block, refused := toolResultOf(t, recs, "ag2turn-orch-b")
	assert.Equal(t, concurrentRefusal, block["content"])
	assert.Equal(t, true, block["is_error"])
	var raw struct {
		ToolUseResult any `json:"toolUseResult"`
	}
	require.NoError(t, json.Unmarshal([]byte(refused.Raw), &raw))
	assert.Equal(t, "Error: "+concurrentRefusal, raw.ToolUseResult)

	want := recordedHooks(t, "bgagent-concurrent-limit")
	pick := func(all []map[string]any) (failure map[string]any, starts int) {
		for _, p := range all {
			switch p["hook_event_name"] {
			case "PostToolUseFailure":
				failure = p
			case "SubagentStart":
				starts++
			}
		}
		return
	}
	failure, starts := pick(got)
	wantFailure, wantStarts := pick(want)
	require.NotNil(t, failure, "the refused call is answered with PostToolUseFailure")
	assert.Equal(t, "Agent", failure["tool_name"])
	assert.Equal(t, concurrentRefusal, wantFailure["error"], "as recorded")
	assert.Equal(t, wantFailure["error"], failure["error"])
	assert.Equal(t, false, failure["is_interrupt"])
	for _, common := range []string{"permission_mode", "prompt_id"} { // every payload's, not this event's
		delete(wantFailure, common)
		delete(failure, common)
	}
	assert.Equal(t, keysOf(wantFailure), keysOf(failure))
	assert.Equal(t, wantStarts, starts, "only the first sub-agent started")
	// the first sub-agent's SubagentStart and its call's PostToolUse race (the
	// recording has them one way, bgagent the other): the order of the rest is pinned
	const first, n = "PreToolUse:Agent", 5
	assert.Equal(t, withoutLabel(fromFirst(hookLabels(want), first, n), "SubagentStart"), withoutLabel(fromFirst(hookLabels(got), first, n+1), "SubagentStart")[:n-1],
		"the hooks in the recorded order: the refused call has a PreToolUse, no SubagentStart")
}

// TestT017_78b_ASubagentMayBeSpawnedOnceTheRunningOneHasFinished: the limit
// counts the sub-agents running: after the first has finished, the next call
// spawns (sub-agents doc: spawning succeeds again when the running count drops).
// sr:proves background-agent/claude
func TestT017_78b_ASubagentMayBeSpawnedOnceTheRunningOneHasFinished(t *testing.T) {
	_, recs := limitRun(t, "lim-2", func(dir string) []string {
		return []string{
			toolUse("ag1", "Agent", `{"prompt":"one","description":"one","run_in_background":true,"script":"`+replyScript(t, dir, "one", "ONE")+`"}`),
			toolUse("b1", "Bash", `{"command":"sleep 1"}`),
			toolUse("ag2", "Agent", `{"prompt":"two","description":"two","script":"`+replyScript(t, dir, "two", "TWO")+`"}`),
		}
	})
	_, second := toolResultOf(t, recs, "ag2turn-orch-c")
	assert.Equal(t, "completed", second.ToolUseResult["status"], "the second call spawned and finished")
}

// withoutLabel is labels less the ones equal to drop.
func withoutLabel(labels []string, drop string) []string {
	var out []string
	for _, l := range labels {
		if l != drop {
			out = append(out, l)
		}
	}
	return out
}

// TestT017_78c_AForegroundCallIsRefusedAtTheLimitToo: the limit is on spawning
// with the Agent tool, whether in the foreground or the background: a foreground
// call made while a sub-agent runs gets the same refusal, with PostToolUseFailure
// and no sub-agent started for it.
// sr:proves background-agent/claude
func TestT017_78c_AForegroundCallIsRefusedAtTheLimitToo(t *testing.T) {
	got, recs := limitRun(t, "lim-3", func(dir string) []string {
		return []string{
			toolUse("ag1", "Agent", `{"prompt":"one","description":"one","run_in_background":true,"script":"`+slowReply(t, dir, "ONE")+`"}`),
			toolUse("ag2", "Agent", `{"prompt":"two","description":"two","script":"`+replyScript(t, dir, "two", "TWO")+`"}`),
		}
	})
	block, _ := toolResultOf(t, recs, "ag2turn-orch-b")
	assert.Equal(t, concurrentRefusal, block["content"])
	assert.Equal(t, []string{"PostToolUseFailure:Agent"}, hookLabelsOf(got, "PostToolUseFailure"))
	assert.Len(t, hookLabelsOf(got, "SubagentStart"), 1)
}

// hookLabelsOf are the labels of the payloads of one event.
func hookLabelsOf(all []map[string]any, event string) []string {
	var out []string
	for i, l := range hookLabels(all) {
		if all[i]["hook_event_name"] == event {
			out = append(out, l)
		}
	}
	return out
}

// TestT017_78d_TheDefaultLimitIsTwenty: unset, the limit is 20 (CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS
// in the env-vars doc, "default: 20"): twenty sub-agents run, and the 21st is
// refused with the limit named.
// sr:proves background-agent/claude
func TestT017_78d_TheDefaultLimitIsTwenty(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	var calls []string
	slow := slowReply(t, dir, "SLOW")
	for i := 0; i < 21; i++ {
		calls = append(calls, toolUse("ag"+string(rune('a'+i)), "Agent", `{"prompt":"p","description":"d`+string(rune('a'+i))+`","run_in_background":true,"script":"`+slow+`"}`))
	}
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "orch", calls...), "--session-id", "lim-4",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "lim-4"))
	refused := strings.ReplaceAll(concurrentRefusal, "1 subagents", "20 subagents")
	for i := 0; i < 21; i++ {
		block, _ := toolResultOf(t, recs, "ag"+string(rune('a'+i))+"turn-orch-"+string(rune('a'+i)))
		if i < 20 {
			assert.NotEqual(t, refused, block["content"], "sub-agent %d spawns", i+1)
			continue
		}
		assert.Equal(t, refused, block["content"], "the 21st is refused")
	}
}
