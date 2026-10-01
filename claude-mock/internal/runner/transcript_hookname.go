package runner

import "github.com/sloprail/harness-mocks/claude-mock/internal/hooks"

// hookRunName is the hookName a fired hook's records carry: the event, and for
// a tool event the tool, for SessionStart its source, for SubagentStart the
// agent type ("PreToolUse:Bash", "SessionStart:startup", "Stop").
func hookRunName(in hooks.Input) string {
	hookName := string(in.HookEventName)
	switch in.HookEventName {
	case hooks.EventPreToolUse, hooks.EventPostToolUse, hooks.EventPostToolUseFailure:
		if in.ToolName != "" {
			hookName += ":" + in.ToolName
		}
	case hooks.EventSessionStart:
		if in.Source != "" {
			hookName += ":" + in.Source
		}
	case hooks.EventSubagentStart:
		if in.AgentType != "" {
			hookName += ":" + in.AgentType
		}
	}
	return hookName
}
