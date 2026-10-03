package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mainThreadLabels are the hooks of the main conversation and the sub-agents'
// ends, by name: what shows when the nested sub-agent reports.
func mainThreadLabels(all []map[string]any) []string {
	var out []string
	for _, p := range all {
		ev, _ := p["hook_event_name"].(string)
		switch {
		case ev == "PostToolUse" && p["tool_name"] == "Agent" && p["agent_id"] == nil:
			out = append(out, "PostToolUse:Agent (main thread)")
		case ev == "SubagentStop", ev == "UserPromptSubmit", ev == "Stop", ev == "SessionEnd":
			out = append(out, ev)
		}
	}
	return out
}

// TestT017_79_NestedBackgroundAgentReportsToTheMainConversation: in a `-p`
// session a sub-agent that launched a background sub-agent does not wait for it
// (sub-agents doc, "Let subagents spawn their own subagents"), and the nested
// one, finishing after its launcher has ended, reports to the main conversation:
// replays the recorded bgagent-nested-launcher run, where the launcher's result
// reaches the main thread first and the first Stop fires, then the nested
// sub-agent ends and its completion notification starts a turn of the main
// conversation (UserPromptSubmit, then a second Stop), and the stream carries
// its report as the task's summary.
// sr:proves background-agent/claude
func TestT017_79_NestedBackgroundAgentReportsToTheMainConversation(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	// the nested sub-agent finishes only once the first Stop has fired
	h := payloadLogger(t, dir, "log.sh", log, `case "$IN" in *'"hook_event_name":"Stop"'*) touch `+dir+`/stopped;; esac`)
	settings(t, dir, allHooks(h))
	inner := write(t, filepath.Join(dir, "inner.sh"), "#!/bin/sh\nwhile [ ! -f "+dir+"/stopped ]; do sleep 0.05; done\nexec sh "+replyScript(t, dir, "innerReply", "INNERREPLY")+"\n", 0o755)
	outer := script(t, dir, "outer", toolUse("out1", "Agent", `{"prompt":"deep","description":"inner","script":"`+inner+`","run_in_background":true}`))
	root := script(t, dir, "root", toolUse("r1", "Agent", `{"prompt":"layer","description":"outer","run_in_background":false,"script":"`+outer+`"}`))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "nest-rep",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	assert.Equal(t, recordedResultStats(t, "bgagent-nested-launcher"), lastResultStats(t, out))
	got, want := payloads(t, log), recordedHooks(t, "bgagent-nested-launcher")
	assert.Equal(t, mainThreadLabels(want), mainThreadLabels(got))
	var notified bool
	for _, p := range got {
		if s, _ := p["prompt"].(string); p["hook_event_name"] == "UserPromptSubmit" && strings.Contains(s, `<summary>Agent "inner" finished</summary>`) {
			notified = true
		}
	}
	assert.True(t, notified, "the nested sub-agent's completion is a turn of the main conversation")
	var summaries []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, `"subtype":"task_notification"`) && strings.Contains(l, `"summary":"INNERREPLY"`) {
			summaries = append(summaries, l)
		}
	}
	assert.Len(t, summaries, 1, "its report reaches the stream, not a kill")
}
