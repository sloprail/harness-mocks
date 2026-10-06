package runner

import (
	"encoding/json"
	"testing"
)

func resultOf(t *testing.T, st *runState, in string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(withResultFields([]byte(in), st, "sid"), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// The result's fields come from what the run did, and the count starts again
// after each result.
func TestResultFieldsFollowTheRun(t *testing.T) {
	var st runState
	st.turn()
	st.deny(pendingToolUse{ToolUseID: "t1", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"x"}`)})
	st.turn()
	first := resultOf(t, &st, `{"type":"result","subtype":"success","result":"a"}`)
	if first["num_turns"] != float64(2) || first["result_index"] != float64(0) || first["stop_reason"] != "end_turn" || first["terminal_reason"] != "completed" {
		t.Fatalf("first: %v", first)
	}
	if d, _ := first["permission_denials"].([]any); len(d) != 1 || d[0].(map[string]any)["tool_use_id"] != "t1" {
		t.Fatalf("denials: %v", first["permission_denials"])
	}
	st.turn()
	second := resultOf(t, &st, `{"type":"result","subtype":"success","result":"b"}`)
	if second["num_turns"] != float64(1) || second["result_index"] != float64(1) || len(second["permission_denials"].([]any)) != 0 {
		t.Fatalf("second: %v", second)
	}
}

// A run in which the model took no turn has no stop reason; an error result's
// API failed, and the script's own fields win.
func TestResultFieldsOfARefusedPromptAndAFailedRun(t *testing.T) {
	refused := resultOf(t, &runState{}, `{"type":"result","subtype":"success","result":"x"}`)
	if refused["stop_reason"] != nil || refused["terminal_reason"] != nil || refused["num_turns"] != float64(0) {
		t.Fatalf("refused: %v", refused)
	}
	var st runState
	st.turn()
	failed := resultOf(t, &st, `{"type":"result","subtype":"success","is_error":true,"api_error_status":404,"result":"x"}`)
	if failed["stop_reason"] != "stop_sequence" || failed["terminal_reason"] != "api_error" || failed["api_error_status"] != float64(404) || failed["is_error"] != true {
		t.Fatalf("failed: %v", failed)
	}
}
