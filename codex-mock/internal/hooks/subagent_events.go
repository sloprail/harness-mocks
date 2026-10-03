package hooks

// The events a sub-agent's life fires (recorded: runs/subagent-lifecycle-hooks).
// Their matcher is applied to the sub-agent's type.
const (
	SubagentStart Event = "SubagentStart"
	SubagentStop  Event = "SubagentStop"
)
