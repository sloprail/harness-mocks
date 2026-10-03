package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_84_NestedBackgroundResultReachesTheMainConversation: in a `-p`
// session, a sub-agent that launched a background sub-agent finishes without
// waiting for it, and the nested one, finishing after its launcher ended,
// reports to the main conversation instead (sub-agents doc, let subagents spawn
// their own subagents): its task_started is at depth 2, its task_notification
// (status completed, its own summary, the call that spawned it) comes after the
// launcher's own and on the main stream.
// sr:proves nested-subagents/claude
func TestT017_84_NestedBackgroundResultReachesTheMainConversation(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	h := payloadLogger(t, dir, "log.sh", filepath.Join(dir, "p.log"), `case "$IN" in *'"hook_event_name":"PostToolUse"'*'"description":"outer"'*) touch `+dir+`/released;; esac`)
	settings(t, dir, map[string]string{"PostToolUse": h})
	inner := write(t, filepath.Join(dir, "inner.sh"), `#!/bin/sh
while [ ! -f `+dir+`/released ]; do sleep 0.05; done
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"INNER-DONE"}]}}'
echo '{"type":"result","subtype":"success","result":"INNER-DONE"}'
`, 0o755)
	outer := script(t, dir, "outer", toolUse("out1", "Agent", `{"prompt":"deep","description":"inner","script":"`+inner+`","run_in_background":true}`))
	root := script(t, dir, "root", toolUse("r1", "Agent", `{"prompt":"layer","description":"outer","script":"`+outer+`"}`))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "nest-rep", "--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	type frame struct{ subtype, task, summary, status string }
	var frames []frame
	started := map[string]float64{}
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil || f["type"] != "system" {
			continue
		}
		sub, _ := f["subtype"].(string)
		task, _ := f["task_id"].(string)
		summary, _ := f["summary"].(string)
		status, _ := f["status"].(string)
		frames = append(frames, frame{sub, task, summary, status})
		assert.Equal(t, "nest-rep", f["session_id"], "every frame is the main conversation's")
		if sub == "task_started" {
			started[f["description"].(string)] = f["spawn_depth"].(float64)
		}
	}
	assert.Equal(t, map[string]float64{"outer": 1, "inner": 2}, started)
	at := func(desc func(frame) bool) int {
		for i, f := range frames {
			if f.subtype == "task_notification" && desc(f) {
				return i
			}
		}
		return -1
	}
	outerDone := at(func(f frame) bool { return f.summary == "done" })
	innerDone := at(func(f frame) bool { return f.summary == "INNER-DONE" })
	require.GreaterOrEqual(t, outerDone, 0, "the launcher's notification")
	require.GreaterOrEqual(t, innerDone, 0, "the nested one's notification reaches the main stream")
	assert.Less(t, outerDone, innerDone, "the launcher ended first; the nested sub-agent reported after it")
	assert.Equal(t, "completed", frames[innerDone].status)
}
