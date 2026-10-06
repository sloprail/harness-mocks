package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay. "adapter:" is something of the recording
// the claude adapter cannot reproduce yet; "mock gap:" is what the mock does not produce that
// the recording shows (each gap is a PR of its own).
var notReplaying = map[string]string{
	"bgagent":                  "adapter: the model said two things before one call: the adapter keeps one",
	"bgagent-concurrent-limit": "adapter: the setup has env, which the adapter does not install",
	"bgagent-definition":       "adapter: the model said two things before one call: the adapter keeps one",
	"bgagent-nested-launcher":  "adapter: the model said two things before one call: the adapter keeps one",
	"cap":                      "adapter: the model said two things before one call: the adapter keeps one",
	"compact":                  "adapter: the setup has then, which the adapter does not install",
	"compact-nohooks":          "adapter: the setup has then, which the adapter does not install",
	"fg-subagent-bash":         "mock gap: assistant frames lack parent_tool_use_id, session_id, wire_tool_inputs; event count 18 vs 14; frame order/kind; hook payloads lack prompt_id/permission_mode or differ in key order",
	"fgsub-maxturns":           "adapter: the model called SendMessage: the adapter maps Bash and Agent",
	"fgsub-tool-stats":         "adapter: sub-agent: the model called Write: the adapter maps Bash and Agent",
	"hook-timeout":             "mock gap: assistant frames lack parent_tool_use_id, session_id, wire_tool_inputs; event count 6 vs 4; frame order/kind; hook payloads lack prompt_id/permission_mode or differ in key order",
	"hookmix":                  "adapter: sub-agent: the model called Read: the adapter maps Bash and Agent",
	"isolated-worktree":        "mock gap: assistant frames lack parent_tool_use_id, session_id, wire_tool_inputs; event count 19 vs 9; frame order/kind; hook payloads lack prompt_id/permission_mode or differ in key order",
	"meta":                     "mock gap: assistant frames lack parent_tool_use_id, session_id, wire_tool_inputs; event count 22 vs 17; frame order/kind; hook payloads lack prompt_id/permission_mode or differ in key order",
	"midturn":                  "mock gap: assistant frames lack parent_tool_use_id, session_id, wire_tool_inputs; result frames lack api_error_status, is_error, num_turns, permission_denials, queued_turn_count, result_index, session_id, stop_reason, terminal_reason, time_to_request_ms, ttft_ms, ttft_stream_ms; user frames lack parent_tool_use_id, session_id, tool_use_result; frame order/kind; hook payloads lack prompt_id/permission_mode or differ in key order",
	"nested-fork-limit":        "adapter: the setup has env, which the adapter does not install",
	"schedule-wakeup-limits":   "adapter: the model called ScheduleWakeup: the adapter maps Bash and Agent",
}
