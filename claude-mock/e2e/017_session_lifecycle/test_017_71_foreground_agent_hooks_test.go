package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_71_NestedAndIsolatedForegroundAgents: replays the recorded meta run
// (an outer foreground sub-agent that dispatches an inner one, then a
// foreground sub-agent with isolation worktree): the hooks fire in the order
// the real claude fired them, every sub-agent's hooks carry its own agent_id
// (the inner Agent call's PreToolUse and PostToolUse the outer's), each
// SubagentStop carries the report as last_assistant_message and an empty
// background_tasks, each Agent call's PostToolUse the report as its content and
// status completed, and the isolated sub-agent starts in
// <project>/.claude/worktrees/agent-<id> and, having left it clean, hands back
// no worktreePath.
// sr:proves foreground-subagent-result/claude
func TestT017_71_NestedAndIsolatedForegroundAgents(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, allHooks(payloadLogger(t, dir, "log.sh", log, "")))
	inner := replyScript(t, dir, "inner", "INNER")
	outer := callThenReply(t, dir, "outer", "OUTER",
		toolUse("in1", "Agent", `{"prompt":"inner","description":"inner","run_in_background":false,"script":"`+inner+`"}`))
	iso := replyScript(t, dir, "iso", "ISO")
	orch := script(t, dir, "orch",
		toolUse("ag1", "Agent", `{"prompt":"outer","description":"outer","run_in_background":false,"script":"`+outer+`"}`),
		toolUse("ag2", "Agent", `{"prompt":"iso","description":"iso","run_in_background":false,"isolation":"worktree","script":"`+iso+`"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "nest-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	assert.Equal(t, recordedResultStats(t, "meta"), lastResultStats(t, out), "the sub-agents counted in the result frame: three, one spawned by a sub-agent, two deep")
	got := payloads(t, log)
	want := recordedHooks(t, "meta")
	assert.Equal(t, hookLabels(want), hookLabels(got), "the hooks fire in the order the real claude fired them")

	var starts, stops, posts []map[string]any
	for _, p := range got {
		switch {
		case p["hook_event_name"] == "SubagentStart":
			starts = append(starts, p)
		case p["hook_event_name"] == "SubagentStop":
			stops = append(stops, p)
		case p["hook_event_name"] == "PostToolUse" && p["tool_name"] == "Agent":
			posts = append(posts, p)
		}
	}
	require.Len(t, starts, 3)
	require.Len(t, stops, 3)
	require.Len(t, posts, 3)
	outerID, innerID, isoID := starts[0]["agent_id"], starts[1]["agent_id"], starts[2]["agent_id"]
	assert.NotEqual(t, outerID, innerID)
	// inner finishes first, then the outer, then the isolated one
	assert.Equal(t, []any{innerID, outerID, isoID}, []any{stops[0]["agent_id"], stops[1]["agent_id"], stops[2]["agent_id"]})
	assert.Equal(t, []any{"INNER", "OUTER", "ISO"},
		[]any{stops[0]["last_assistant_message"], stops[1]["last_assistant_message"], stops[2]["last_assistant_message"]})
	for _, s := range stops {
		assert.Equal(t, []any{}, s["background_tasks"])
		assert.Equal(t, false, s["stop_hook_active"])
		assert.Equal(t, "general-purpose", s["agent_type"])
	}
	// the inner Agent call is the outer sub-agent's own call
	assert.Equal(t, outerID, posts[0]["agent_id"])
	assert.Nil(t, posts[1]["agent_id"])
	assert.Equal(t, innerID, posts[0]["tool_response"].(map[string]any)["agentId"])
	for i, report := range []string{"INNER", "OUTER", "ISO"} {
		r := posts[i]["tool_response"].(map[string]any)
		assert.Equal(t, "completed", r["status"])
		assert.Equal(t, []any{map[string]any{"type": "text", "text": report}}, r["content"])
		assert.Equal(t, keysOf(agentPosts(want)[i]["tool_response"].(map[string]any)), keysOf(r), "the fields of the real tool_response")
	}
	assert.EqualValues(t, 0, posts[0]["tool_response"].(map[string]any)["totalToolUseCount"])
	assert.EqualValues(t, 1, posts[1]["tool_response"].(map[string]any)["totalToolUseCount"], "the outer sub-agent made one call: the inner Agent")

	// the isolated sub-agent runs in its own worktree; the others in the project
	project := got[0]["cwd"]
	assert.Equal(t, project, starts[0]["cwd"])
	assert.Equal(t, filepath.Join(project.(string), ".claude", "worktrees", "agent-"+isoID.(string)), starts[2]["cwd"])
	assert.NotContains(t, posts[2]["tool_response"], "worktreePath", "a clean worktree is gone, and the hand-back names none")
}

// TestT017_72_SubagentStopBlockedOnceReRunsTheForegroundAgent: replays the
// recorded hookmix run (a SubagentStop hook that blocks by exit 2 the first
// time): SubagentStop fires again, stop_hook_active true, once the sub-agent
// has acted on the hook's feedback (here, a Read before its second reply); the
// Agent call returns only then, its PostToolUse after the second SubagentStop.
// The hooks fire in the recorded order and the sub-agent's own transcript holds
// the feedback as a user turn.
// sr:proves foreground-subagent-result/claude
func TestT017_72_SubagentStopBlockedOnceReRunsTheForegroundAgent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	count := filepath.Join(dir, "stops")
	h := payloadLogger(t, dir, "log.sh", log, `case "$IN" in *'"hook_event_name":"SubagentStop"'*)
M=$(cat `+count+` 2>/dev/null || echo 0); M=$((M+1)); echo $M > `+count+`
if [ $M = 1 ]; then echo SUB-BLOCK >&2; exit 2; fi;; esac`)
	settings(t, dir, allHooks(h))
	reply := replyScript(t, dir, "reply", "HELPED")
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
F="$A10N_MOCK_SESSION_FILE"
if grep -q SUB-BLOCK "$F" && ! grep -q mrdturn "$F"; then
cat <<'JSONL'
`+strings.Replace(toolUse("rd1", "Read", `{"file_path":"`+filepath.Join(dir, "log.sh")+`"}`), "@MARK@", "mrdturn", 1)+`
JSONL
exit 0
fi
exec sh `+reply+`
`, 0o755)
	orch := script(t, dir, "orch",
		toolUse("b1", "Bash", `{"command":"echo SECOND"}`),
		toolUse("ag1", "Agent", `{"prompt":"go","description":"helper","script":"`+sub+`"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "blk-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	got := payloads(t, log)
	assert.Equal(t, hookLabels(recordedHooks(t, "hookmix")), hookLabels(got))
	var stops []map[string]any
	for _, p := range got {
		if p["hook_event_name"] == "SubagentStop" {
			stops = append(stops, p)
		}
	}
	require.Len(t, stops, 2)
	assert.Equal(t, false, stops[0]["stop_hook_active"])
	assert.Equal(t, true, stops[1]["stop_hook_active"])
	assert.Equal(t, "HELPED", stops[1]["last_assistant_message"])
	post := agentPosts(got)[0]["tool_response"].(map[string]any)
	assert.Equal(t, []any{map[string]any{"type": "text", "text": "HELPED"}}, post["content"])
	assert.EqualValues(t, 1, post["totalToolUseCount"])

	raw, err := os.ReadFile(stops[1]["agent_transcript_path"].(string))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `Stop hook feedback:\n[`+filepath.Join(dir, "log.sh")+`]: SUB-BLOCK\n`)
}
