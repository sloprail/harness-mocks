package e2e

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookLabels are what a hook log shows of a run, one label per line: the event of a
// payload, or the hook that logged its own result; sorted, since hooks of one event
// run at the same time.
func hookLabels(lines []map[string]any) (out []string) {
	for _, l := range lines {
		switch {
		case l["hook_event_name"] != nil:
			label := fmt.Sprint(l["hook_event_name"])
			if a, ok := l["stop_hook_active"]; ok {
				label += fmt.Sprintf(" active=%v", a)
			}
			out = append(out, label)
		case l["hook_result"] != nil:
			r := l["hook_result"].(map[string]any)
			out = append(out, fmt.Sprintf("result %v", r["hook"]))
		}
	}
	sort.Strings(out)
	return
}

// runSubagentStop replays a recorded run's hooks on a spawn, a wait and an answer,
// the sub-agent answering PONG.
func runSubagentStop(t *testing.T, name string) (recorded, got []map[string]any) {
	t.Helper()
	rec := loadRecording(t, name)
	recorded = jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	r := execMock(t, scenario{Script: pingSpawn,
		Files:     map[string]string{"pong.sh": pongScript, "hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")), Prompt: "go"})
	require.Equal(t, 0, r.Code, r.Stderr)
	return recorded, r.hookLog()
}

// continue:false of one SubagentStop hook takes precedence over another's block: the
// sub-agent is not continued, and both hooks ran (recorded: runs/subagent-stop-continue-false).
// sr:proves hook-exit-code-semantics/codex
// sr:proves subagent-lifecycle-hooks/codex
func TestASubagentStopContinueFalseBeatsAnotherHooksBlock(t *testing.T) {
	recorded, got := runSubagentStop(t, "subagent-stop-continue-false")
	assert.Equal(t, hookLabels(recorded), hookLabels(got))
	assert.Zero(t, countLabel(hookLabels(got), "SubagentStop active=true"), "the sub-agent was stopped, not run again")
}

// Plain text on stdout with exit 0 is invalid for SubagentStop: it is ignored and the
// sub-agent stops (recorded: runs/subagent-stop-plain-text).
// sr:proves hook-exit-code-semantics/codex
// sr:proves subagent-lifecycle-hooks/codex
func TestASubagentStopPlainTextIsIgnored(t *testing.T) {
	recorded, got := runSubagentStop(t, "subagent-stop-plain-text")
	assert.Equal(t, hookLabels(recorded), hookLabels(got))
	assert.Zero(t, countLabel(hookLabels(got), "SubagentStop active=true"), "the sub-agent was stopped, not continued")
}

func countLabel(labels []string, want string) (n int) {
	for _, l := range labels {
		if l == want {
			n++
		}
	}
	return
}
