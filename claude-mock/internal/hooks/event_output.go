package hooks

import (
	"encoding/json"
)

// BackgroundTask is one entry of a Stop/SubagentStop payload's
// background_tasks: a task still running when the hook fires. The shapes are
// the ones claude 2.1.282 sent (a background Bash is type "shell" with its
// command; a background Agent is type "subagent" with its agent_type).
type BackgroundTask struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
}

// Output is the JSON response a hook handler may write to stdout.
// All fields are optional; unset fields have no effect.
type Output struct {
	// PlainText is stdout that is not JSON (see internal/hooks.IsJSONOutput):
	// UserPromptSubmit and SessionStart take it as context.
	PlainText     string `json:"-"`
	Continue      *bool  `json:"continue,omitempty"`
	StopReason    string `json:"stopReason,omitempty"`
	SystemMessage string `json:"systemMessage,omitempty"`
	// Decision / Reason are the documented Stop / UserPromptSubmit block controls.
	// Decision "block" + Reason keeps the agent working (Stop) or rejects the
	// prompt (UserPromptSubmit). For PreToolUse these top-level fields are
	// DEPRECATED — use HookSpecificOutput.PermissionDecision instead.
	// sr:docs https://code.claude.com/docs/en/hooks#stop
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`

	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput carries event-specific control fields inside Output.
type HookSpecificOutput struct {
	HookEventName            EventName       `json:"hookEventName,omitempty"`
	AdditionalContext        string          `json:"additionalContext,omitempty"`
	PermissionDecision       string          `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string          `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             json.RawMessage `json:"updatedInput,omitempty"`
	UpdatedToolOutput        string          `json:"updatedToolOutput,omitempty"`
	WorktreePath             string          `json:"worktreePath,omitempty"`
}

// isOutputField reports whether key is a top-level field of Claude Code's hook
// JSON output (the docs' "JSON output" table).
// sr:docs https://code.claude.com/docs/en/hooks#json-output
func isOutputField(key string) bool {
	switch key {
	case "continue", "suppressOutput", "stopReason", "decision", "reason", "systemMessage", "terminalSequence", "hookSpecificOutput":
		return true
	}
	return false
}
