package runner

import (
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// withResultFields is a result frame carrying what the real claude's result
// frame carries beside the script's own fields (recorded: every run's result
// frames), derived from what the run did: num_turns (the model's turns since
// the last result), permission_denials (the calls a hook refused),
// result_index (the results before it) and, for a run in which the model took
// a turn, stop_reason end_turn and terminal_reason completed (stop_reason null and
// no terminal_reason or api_error_status when it took none: a refused prompt, a compaction). A frame that is an error says its API
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
	if turns == 0 && !failed {
		// a run in which the model took no turn has neither (recorded: prompt-blocked, compact, resume-unknown)
		delete(fields, "terminal_reason")
		delete(fields, "api_error_status")
	}
	if origin := st.takeOrigin(); origin != nil {
		fields["origin"] = origin
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

// finisher is what writes the run's held result frame to the stream, ending the
// run: the run's own result frames carry the run's state, a sub-agent's (internal) do not.
func finisher(cfg Config, bg *backgroundTasks, final *scenario.Result) func() {
	return func() {
		if bg.refused.Err() != nil { // a run that refused a call streams no result: it fails
			return
		}
		final.Finish(func(line []byte) {
			if cfg.AgentID == "" {
				line = withResultFields(line, &bg.run, cfg.SessionID)
				bg.writeResult(cfg, line)
				return
			}
			writeStreamLine(cfg, line)
		})
	}
}

// maxTurnsReached ends a run that has taken --max-turns model turns and would
// take another: it streams the error result real claude gives (recorded:
// snapshots/runs/max-turns: subtype error_max_turns, num_turns one past the
// limit, stop_reason of the last turn, terminal_reason max_turns, the error
// text, no result text; no Stop fires, SessionEnd does, exit 1) and reports it.
func maxTurnsReached(cfg Config, bg *backgroundTasks) bool {
	if cfg.MaxTurns <= 0 || cfg.AgentID != "" {
		return false
	}
	turns, denials, index := bg.run.take()
	if turns < cfg.MaxTurns {
		bg.run.restore(turns, denials, index)
		return false
	}
	list := make([]any, len(denials))
	for i, d := range denials {
		list[i] = d
	}
	line, err := marshalRecord(map[string]any{
		"type": "result", "subtype": "error_max_turns", "is_error": true, "num_turns": turns + 1, "stop_reason": "tool_use",
		"terminal_reason": "max_turns", "errors": []string{fmt.Sprintf("Reached maximum number of turns (%d)", cfg.MaxTurns)},
		"permission_denials": list, "queued_turn_count": 0, "result_index": index, "session_id": cfg.SessionID,
	})
	if err == nil {
		writeStreamLine(cfg, withSubagentStats(line, bg))
	}
	return true
}
