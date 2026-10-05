package runner

import (
	"encoding/json"
	"sync"
)

// runState is what the run has done since its last result frame, from which the
// next result's fields are derived: the turns the main agent's model took, the
// tool calls a hook refused, and how many results came before.
type runState struct {
	mu      sync.Mutex
	turns   int
	denials []map[string]any
	results int
}

// turn counts a turn the model took: it called a tool or answered.
func (r *runState) turn() {
	r.mu.Lock()
	r.turns++
	r.mu.Unlock()
}

// deny records a tool call a PreToolUse hook refused (the result's permission_denials).
func (r *runState) deny(call pendingToolUse) {
	var input any
	_ = json.Unmarshal(call.ToolInput, &input)
	r.mu.Lock()
	r.denials = append(r.denials, map[string]any{"tool_name": call.ToolName, "tool_use_id": call.ToolUseID, "tool_input": input})
	r.mu.Unlock()
}

// take is the state for the result being written, and starts the next
// result's count from nothing.
func (r *runState) take() (turns int, denials []map[string]any, index int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	turns, denials, index = r.turns, r.denials, r.results
	r.turns, r.denials, r.results = 0, nil, r.results+1
	return
}

// withResultFields is a result frame carrying what the real claude's result
// frame carries beside the script's own fields (recorded: every run's result
// frames), derived from what the run did: num_turns (the model's turns since
// the last result), permission_denials (the calls a hook refused),
// result_index (the results before it) and, for a run in which the model took
// a turn, stop_reason end_turn and terminal_reason completed (null when it took
// none: a refused prompt, a compaction). A frame that is an error says its API
// failed: stop_reason stop_sequence and terminal_reason api_error, with the
// script's own api_error_status (recorded: snapshots/runs/run-failure). The
// script's own fields win.
func withResultFields(line []byte, st *runState, sessionID string) []byte {
	var frame map[string]any
	if json.Unmarshal(line, &frame) != nil || frame["type"] != "result" || len(line) == 0 || line[0] != '{' {
		return line
	}
	turns, denials, index := st.take()
	failed, _ := frame["is_error"].(bool)
	stop, terminal := any("end_turn"), any("completed")
	switch {
	case failed:
		stop, terminal = "stop_sequence", "api_error"
	case turns == 0:
		stop, terminal = nil, nil
	}
	list := make([]any, len(denials))
	for i, d := range denials {
		list[i] = d
	}
	fields := map[string]any{
		"is_error": false, "num_turns": turns, "stop_reason": stop, "terminal_reason": terminal,
		"permission_denials": list, "queued_turn_count": 0, "result_index": index, "api_error_status": nil,
		"session_id": sessionID,
	}
	for k, v := range fields {
		if _, ok := frame[k]; !ok {
			frame[k] = v
		}
	}
	out, err := marshalRecord(frame)
	if err != nil {
		return line
	}
	return out
}
