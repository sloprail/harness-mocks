package replay

import "testing"

// The first beforeShellExecution's transcript path is marked on both sides, and
// nothing after that event's own hook run is touched.
func TestUnsettledMarksOnlyTheFirstBeforeShellExecution(t *testing.T) {
	log := []map[string]any{
		{"hook_event_name": "preToolUse", "transcript_path": nil},
		{"hook_event_name": "beforeShellExecution", "transcript_path": nil},
		{"hook_env": map[string]any{"CURSOR_VERSION": "v"}},
		{"hook_event_name": "beforeShellExecution", "transcript_path": "/t"},
		{"hook_env": map[string]any{}},
	}
	unsettled(log)
	if log[0]["transcript_path"] != nil || log[1]["transcript_path"] != "<unsettled>" || log[3]["transcript_path"] != "/t" {
		t.Fatalf("paths: %v", log)
	}
	if log[2]["hook_env"].(map[string]any)["CURSOR_TRANSCRIPT_PATH"] != "<unsettled>" || len(log[4]["hook_env"].(map[string]any)) != 0 {
		t.Fatalf("env: %v", log)
	}
}
