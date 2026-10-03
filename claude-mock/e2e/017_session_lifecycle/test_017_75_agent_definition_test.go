package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentDefinition writes a sub-agent definition into the project.
func agentDefinition(t *testing.T, dir, name, frontmatter string) {
	t.Helper()
	write(t, filepath.Join(dir, ".claude", "agents", name+".md"),
		"---\nname: "+name+"\ndescription: test agent\n"+frontmatter+"---\nReply.\n", 0o644)
}

// upTo are the labels up to and including the first Agent PostToolUse.
func upTo(labels []string) []string {
	for i, l := range labels {
		if l == "PostToolUse:Agent" {
			return labels[:i+1]
		}
	}
	return labels
}

// TestT017_75_DefinitionBackgroundKeepsAForegroundCallInTheBackground: a
// sub-agent whose definition sets `background: true` runs in the background even
// when Claude asks for the foreground, in -p mode (sub-agents doc, "Run
// subagents in foreground or background"): replays the recorded
// bgagent-definition run, where a call with run_in_background false is answered
// with the async_launched receipt, the sub-agent is started in the background
// and its completion arrives as a notification. A definition that does not set
// it, and CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1, leave the call in the foreground.
// sr:proves background-agent/claude
func TestT017_75_DefinitionBackgroundKeepsAForegroundCallInTheBackground(t *testing.T) {
	call := func(t *testing.T, frontmatter string, env []string) (string, []map[string]any) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		log := filepath.Join(dir, "payloads.log")
		settings(t, dir, allHooks(payloadLogger(t, dir, "log.sh", log, "")))
		agentDefinition(t, dir, "bgdef", frontmatter)
		sub := replyScript(t, dir, "sub", "BGDEFREPLY")
		orch := script(t, dir, "orch", toolUse("ag1", "Agent",
			`{"prompt":"go","description":"bgdef","subagent_type":"bgdef","run_in_background":false,"script":"`+sub+`"}`))
		out, code := runInDir(t, dir, env, "--script", orch, "--session-id", "def-1",
			"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
		require.Equal(t, 0, code, out)
		return out, payloads(t, log)
	}

	out, got := call(t, "background: true\n", nil)
	want := recordedHooks(t, "bgagent-definition")
	assert.Equal(t, upTo(hookLabels(want)), upTo(hookLabels(got)), "the hooks up to the call's answer, as recorded")
	post := agentPosts(got)[0]["tool_response"].(map[string]any)
	assert.Equal(t, "async_launched", post["status"])
	assert.Equal(t, true, post["isAsync"])
	assert.Equal(t, keysOf(agentPosts(want)[0]["tool_response"].(map[string]any)), keysOf(post))
	assert.Equal(t, "bgdef", agentPosts(got)[0]["tool_input"].(map[string]any)["subagent_type"])

	frames := framesOf(t, out, post["agentId"].(string))
	require.NotEmpty(t, frames)
	assert.Equal(t, "task_started", frames[0]["subtype"])
	assert.Equal(t, true, frames[0]["is_backgrounded"])
	assert.Equal(t, "bgdef", frames[0]["subagent_type"])
	last := frames[len(frames)-1]
	assert.Equal(t, "task_notification", last["subtype"], "its completion is announced, as recorded")
	assert.Equal(t, "completed", last["status"])
	assert.Equal(t, "BGDEFREPLY", last["summary"])
	var notified bool
	for _, p := range got {
		if s, _ := p["prompt"].(string); p["hook_event_name"] == "UserPromptSubmit" && strings.Contains(s, `<summary>Agent "bgdef" finished</summary>`) {
			notified = true
		}
	}
	assert.True(t, notified, "and starts a turn")
	// two turns, as recorded: a result for each, and the session ends after the second Stop
	raw, err := os.ReadFile(filepath.Join(recordedSample(t, "bgagent-definition"), "stream.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, strings.Count(string(raw), `"type":"result"`), strings.Count(out, `"type":"result"`), "a result for each turn")
	labels := hookLabels(got)
	assert.Equal(t, []string{"Stop", "SessionEnd"}, labels[len(labels)-2:], "the session ends after the last Stop")
	assert.Equal(t, hookLabels(want)[len(hookLabels(want))-2:], labels[len(labels)-2:])

	for name, c := range map[string]struct {
		frontmatter string
		env         []string
	}{
		"definition without background":    {"model: haiku\n", nil},
		"definition with background false": {"background: false\n", nil},
		"background tasks disabled":        {"background: true\n", []string{"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, got := call(t, c.frontmatter, c.env)
			assert.Equal(t, "completed", agentPosts(got)[0]["tool_response"].(map[string]any)["status"])
		})
	}
}

// TestT017_76_MaxTurnsStopsTheAgentWithANote: a sub-agent whose definition sets
// `maxTurns` stops at the limit (Agent tool behavior, tools reference): replays
// the recorded fgsub-maxturns run, a sub-agent with maxTurns 2 asked to run four
// commands, which ran two, fired no SubagentStop, and whose Agent call returned
// with only the note that it stopped at its 2-turn limit with no report, as one
// harness note (harnessNoteCount 1, the note the only content block) and the
// recorded hash.
// sr:proves foreground-subagent-result/claude
func TestT017_76_MaxTurnsStopsTheAgentWithANote(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, allHooks(payloadLogger(t, dir, "log.sh", log, "")))
	agentDefinition(t, dir, "capped", "maxTurns: 2\n")
	sub := callThenReply(t, dir, "sub", "ALLDONE",
		toolUse("c1", "Bash", `{"command":"echo ONE"}`), toolUse("c2", "Bash", `{"command":"echo TWO"}`),
		toolUse("c3", "Bash", `{"command":"echo THREE"}`), toolUse("c4", "Bash", `{"command":"echo FOUR"}`))
	orch := script(t, dir, "orch", toolUse("ag1", "Agent",
		`{"prompt":"go","description":"capped","subagent_type":"capped","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "cap-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	got := payloads(t, log)
	want := recordedHooks(t, "fgsub-maxturns")
	assert.Equal(t, upTo(hookLabels(want)), upTo(hookLabels(got)), "no SubagentStop: the sub-agent was cut off")
	assert.NotContains(t, hookLabels(got), "SubagentStop")
	gr, wr := agentPosts(got)[0]["tool_response"].(map[string]any), agentPosts(want)[0]["tool_response"].(map[string]any)
	assert.Equal(t, wr["content"], gr["content"])
	assert.Equal(t, wr["harnessNoteCount"], gr["harnessNoteCount"])
	assert.Equal(t, wr["harnessSectionHash"], gr["harnessSectionHash"])
	assert.Equal(t, wr["totalToolUseCount"], gr["totalToolUseCount"])
	assert.Equal(t, wr["toolStats"], gr["toolStats"])
	assert.Equal(t, keysOf(wr), keysOf(gr))
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "cap-1")), "ag1turn-orch-a")
	text := block["content"].([]any)[0].(map[string]any)["text"].(string)
	assert.Equal(t, agentTrailer.ReplaceAllString(recordedAgentResultText(t, "fgsub-maxturns"), ""), agentTrailer.ReplaceAllString(text, ""))
	assert.NotContains(t, text, "THREE")
}
