package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedRefusal is what the real claude answered the Agent call refused for the
// concurrent limit with (recorded: bgagent-concurrent-limit, run with
// CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS=1), as it reads at the default limit of 20.
func recordedRefusal(t *testing.T) string {
	t.Helper()
	for _, p := range recordedHooks(t, "bgagent-concurrent-limit") {
		if p["hook_event_name"] == "PostToolUseFailure" {
			msg := p["error"].(string)
			require.Contains(t, msg, "You can run 1 subagents at once.")
			return strings.Replace(msg, "1 subagents", "20 subagents", 1)
		}
	}
	t.Fatal("the recording has no refused call")
	return ""
}

// agentCalls are n background Agent calls, ids ag<a..>, that run until the file
// released exists, then last.
func agentCalls(t *testing.T, dir, released string, n int, last string) []string {
	t.Helper()
	slow := write(t, filepath.Join(dir, "slow.sh"), "#!/bin/sh\nwhile [ ! -f "+released+" ]; do sleep 0.05; done\nexec sh "+replyScript(t, dir, "slowReply", "SLOW")+"\n", 0o755)
	var calls []string
	for i := 0; i < n; i++ {
		calls = append(calls, toolUse("ag"+string(rune('a'+i)), "Agent", `{"prompt":"p","description":"d`+string(rune('a'+i))+`","run_in_background":true,"script":"`+slow+`"}`))
	}
	return append(calls, last)
}

// releasedBy is a hook that lets the sub-agents finish once the call with this
// tool_use id has been answered, whatever the machine's speed.
func releasedBy(t *testing.T, dir, released, idPrefix string) string {
	t.Helper()
	return payloadLogger(t, dir, "log.sh", filepath.Join(dir, "payloads.log"),
		`case "$IN" in *'"tool_use_id":"`+idPrefix+`'*) touch `+released+`;; esac`)
}

// TestT017_78_ConcurrentSubagentLimit: with twenty sub-agents running, spawning
// another with the Agent tool fails with `Concurrent subagent limit reached`
// (sub-agents doc, "Concurrent subagent limit"), in the background or the
// foreground: replays what the recorded bgagent-concurrent-limit run (its limit
// set to 1) shows, the call answered with the tool error telling Claude not to
// retry (is_error, toolUseResult "Error: <message>"), PreToolUse fired and
// PostToolUseFailure answering it with that message and is_interrupt false, and
// no SubagentStart for it. The twenty before it spawned.
// sr:proves background-agent/claude
func TestT017_78_ConcurrentSubagentLimit(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	released := filepath.Join(dir, "released")
	// no SubagentStop hook: twenty sub-agents ending at once would interleave the log
	rel := releasedBy(t, dir, released, "agv")
	settings(t, dir, map[string]string{"PreToolUse": rel, "PostToolUse": rel, "PostToolUseFailure": rel, "SubagentStart": rel})
	foreground := toolUse("agv", "Agent", `{"prompt":"p","description":"fg","script":"`+replyScript(t, dir, "fg", "FG")+`"}`)
	calls := agentCalls(t, dir, released, 21, foreground)
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "orch", calls...), "--session-id", "lim-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "lim-1"))
	refusal := recordedRefusal(t)
	for i := 0; i < 22; i++ {
		id := "ag" + string(rune('a'+i))
		block, r := toolResultOf(t, recs, id+"turn-orch-"+string(rune('a'+i)))
		if i < 20 {
			assert.NotEqual(t, refusal, block["content"], "sub-agent %d spawns", i+1)
			continue
		}
		assert.Equal(t, refusal, block["content"], "call %d is refused, the 21st in the background and the 22nd in the foreground", i+1)
		assert.Equal(t, true, block["is_error"])
		var raw struct {
			ToolUseResult any `json:"toolUseResult"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &raw))
		assert.Equal(t, "Error: "+refusal, raw.ToolUseResult)
	}

	var failures []map[string]any
	starts := 0
	for _, p := range payloads(t, log) {
		switch p["hook_event_name"] {
		case "PostToolUseFailure":
			failures = append(failures, p)
		case "SubagentStart":
			starts++
		}
	}
	require.Len(t, failures, 2, "each refused call is answered with PostToolUseFailure")
	var wantFailure map[string]any
	for _, p := range recordedHooks(t, "bgagent-concurrent-limit") {
		if p["hook_event_name"] == "PostToolUseFailure" {
			wantFailure = p
		}
	}
	for _, common := range []string{"permission_mode", "prompt_id"} { // every payload's, not this event's
		delete(wantFailure, common)
	}
	for _, f := range failures {
		for _, common := range []string{"permission_mode", "prompt_id"} {
			delete(f, common)
		}
		assert.Equal(t, "Agent", f["tool_name"])
		assert.Equal(t, refusal, f["error"])
		assert.Equal(t, false, f["is_interrupt"])
		assert.Equal(t, keysOf(wantFailure), keysOf(f))
	}
	assert.Equal(t, 20, starts, "only the twenty before the limit started")
	stats := lastResultStats(t, out)
	assert.Equal(t, map[string]any{"depth_limit": 0.0, "concurrency_limit": 2.0, "budget": 0.0}, stats["refused"], "the two refusals are counted, as the recorded run counted its one")
	assert.EqualValues(t, 20, stats["spawned"])
	assert.Equal(t, keysOf(recordedResultStats(t, "bgagent-concurrent-limit")), keysOf(stats))
}

// TestT017_78b_ASubagentMayBeSpawnedOnceRunningOnesHaveFinished: the limit counts
// the sub-agents running: after they have finished, the next call spawns
// (sub-agents doc: spawning succeeds again when the running count drops).
// sr:proves background-agent/claude
func TestT017_78b_ASubagentMayBeSpawnedOnceRunningOnesHaveFinished(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	released := filepath.Join(dir, "released")
	settings(t, dir, map[string]string{"PostToolUse": releasedBy(t, dir, released, "agt")})
	next := toolUse("next", "Agent", `{"prompt":"p","description":"next","script":"`+replyScript(t, dir, "next", "NEXT")+`"}`)
	calls := agentCalls(t, dir, released, 20, toolUse("wait", "Bash", `{"command":"sleep 2"}`))
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "orch", append(calls, next)...), "--session-id", "lim-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	_, spawned := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "lim-2")), "nextturn-orch-v")
	assert.Equal(t, "completed", spawned.ToolUseResult["status"], "the call after the twenty had finished spawned")
}
