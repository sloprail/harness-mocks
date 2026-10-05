package runner

import "encoding/json"

// withResultFields is a result frame carrying what the real claude's result
// frame carries beside the script's own fields (recorded: every run's last
// frame): how many turns the model took (num_turns), why the last one ended
// (stop_reason end_turn and terminal_reason completed; null for a run in which
// the model took none: a refused prompt, a compaction), the permission denials
// (none: the mock denies no tool), the queued turns (none), the index of the
// result in the run (0) and whether the API failed (it does not). The script's
// own fields win.
func withResultFields(line []byte, turns int64, sessionID string) []byte {
	var frame map[string]any
	if json.Unmarshal(line, &frame) != nil || frame["type"] != "result" || len(line) == 0 || line[0] != '{' {
		return line
	}
	stop, terminal := any("end_turn"), any("completed")
	if turns == 0 {
		stop, terminal = nil, nil
	}
	fields := map[string]any{
		"is_error": false, "num_turns": turns, "stop_reason": stop, "terminal_reason": terminal,
		"permission_denials": []any{}, "queued_turn_count": 0, "result_index": 0, "api_error_status": nil,
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
