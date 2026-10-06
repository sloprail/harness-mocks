package replay

import (
	"path/filepath"
	"testing"
)

func frame(kind, parent, name string) map[string]any {
	f := map[string]any{"type": kind, "parent_tool_use_id": nil}
	if parent != "" {
		f["parent_tool_use_id"] = parent
	}
	if name != "" {
		f["message"] = map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "call", "name": name}}}
	}
	return f
}

// A sub-agent's shell is carried out early when no message of another agent sits between its call and its
// task_started in the stream, late when one does; the samples of one run can differ (the harness races).
func TestExecEarlyFollowsTheStreamsOrder(t *testing.T) {
	started := map[string]any{"subtype": "task_started", "task_type": "local_bash", "owned_by_subagent": true, "tool_use_id": "call"}
	text := map[string]any{"type": "assistant", "parent_tool_use_id": nil, "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "LAUNCHED"}}}}
	early := execEarly([]map[string]any{frame("assistant", "sub", "Bash"), started})
	late := execEarly([]map[string]any{frame("assistant", "sub", "Bash"), text, started})
	if !early["call"] || late["call"] {
		t.Fatalf("early %v late %v", early, late)
	}
}

// The recorded samples of bgagent-concurrent-limit show both orders of the sub-agent's shell and of
// SubagentStart against the launching call's PostToolUse; each sample's own is read.
func TestTheSamplesOfARacyRunEachCarryTheirOwnOrder(t *testing.T) {
	run := filepath.Join("..", "..", "snapshots", "runs", "bgagent-concurrent-limit")
	var earlies, lates int
	for _, s := range sampleDirs(run) {
		rec, err := Adapter{}.LoadSample(run, s)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range rec.Agent.Calls {
			if c.Sub != nil {
				for _, sc := range c.Sub.Calls {
					if sc.ExecEarly {
						earlies++
					}
				}
			}
			if c.Input["mock_start_after_post"] == true {
				lates++
			}
		}
	}
	if earlies != 1 || lates != 2 {
		t.Fatalf("one sample runs the shell early and two start the sub-agent after the post: %d %d", earlies, lates)
	}
}
