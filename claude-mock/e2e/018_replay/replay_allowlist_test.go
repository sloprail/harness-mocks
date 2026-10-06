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
	"meta":                     "mock gap: assistant frames lack parent_tool_use_id, session_id, wire_tool_inputs; event count 22 vs 17; frame order/kind; hook payloads lack prompt_id/permission_mode or differ in key order",
	"nested-fork-limit":        "adapter: the setup has env, which the adapter does not install",
	"schedule-wakeup-limits":   "adapter: the model called ScheduleWakeup: the adapter maps Bash and Agent",
}
