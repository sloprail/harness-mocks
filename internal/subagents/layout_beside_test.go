package subagents

import "testing"

// A harness with no sub-agent directory keeps the sub-agent's transcript beside
// the session's file, or in a directory of its own beside the session's.
func TestLayoutWithoutSubagentDirectory(t *testing.T) {
	file := Layout{Prefix: "rollout-", Ext: ".jsonl"}
	if got, want := file.Path("/s/2026/rollout-a.jsonl", "c1"), "/s/2026/rollout-c1.jsonl"; got != want {
		t.Errorf("beside the file: %s, want %s", got, want)
	}
	dir := Layout{Ext: ".jsonl", OwnDir: true}
	if got, want := dir.Path("/t/s1/s1.jsonl", "c1"), "/t/c1/c1.jsonl"; got != want {
		t.Errorf("own directory: %s, want %s", got, want)
	}
}
