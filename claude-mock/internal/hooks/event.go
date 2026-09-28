// Package hooks defines the Claude Code hook event types and the settings-file
// schema used to configure which hooks fire for which events.
package hooks

import "encoding/json"

// EventName identifies a Claude Code lifecycle hook event.
type EventName string

const (
	EventSessionStart  EventName = "SessionStart"
	EventSessionEnd    EventName = "SessionEnd"
	EventStop          EventName = "Stop"
	EventSubagentStart EventName = "SubagentStart"
	EventSubagentStop  EventName = "SubagentStop"
	EventStopFailure   EventName = "StopFailure"
	EventPreToolUse    EventName = "PreToolUse"
	EventPostToolUse   EventName = "PostToolUse"
	// EventPostToolUseFailure fires instead of PostToolUse when a tool that ran
	// failed (claude 2.1.282: a failing foreground Bash fired it, with error).
	// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
	EventPostToolUseFailure EventName = "PostToolUseFailure"
	EventWorktreeCreate     EventName = "WorktreeCreate"
	EventWorktreeRemove     EventName = "WorktreeRemove"
	// EventPreCompact / EventPostCompact bracket a compaction. PreCompact can
	// block it (exit 2 or decision:block); PostCompact receives the summary.
	// sr:docs https://code.claude.com/docs/en/hooks#precompact
	// sr:docs https://code.claude.com/docs/en/hooks#postcompact
	EventPreCompact  EventName = "PreCompact"
	EventPostCompact EventName = "PostCompact"
	// EventUserPromptSubmit fires before each user message is sent to the model.
	// Per the real Claude Code contract the hook CANNOT replace the prompt: it
	// may only append "additionalContext" (inside hookSpecificOutput), or block
	// the prompt via {"decision":"block","reason":...} / exit 2. There is no
	// "userPrompt"/"updatedPrompt" field (that is an open upstream feature
	// request, anthropics/claude-code#27365). If the hook outputs nothing, the
	// original prompt is forwarded unchanged.
	// sr:docs https://code.claude.com/docs/en/hooks#userpromptsubmit
	EventUserPromptSubmit EventName = "UserPromptSubmit"
)

// Input is the JSON payload sent to a hook handler via stdin (command hooks)
// or POST body (HTTP hooks). Fields are populated per event type.
type Input struct {
	SessionID      string    `json:"session_id"`
	TranscriptPath string    `json:"transcript_path,omitempty"`
	Cwd            string    `json:"cwd"`
	HookEventName  EventName `json:"hook_event_name"`
	PermissionMode string    `json:"permission_mode,omitempty"`

	// SessionStart: Source is "startup" | "resume" | "clear" | "compact" |
	// "fork". Verified against claude 2.1.282: a `--resume <id> --fork-session`
	// run fires SessionStart with source "fork" (docs: SessionStart input).
	// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
	Source string `json:"source,omitempty"`
	Model  string `json:"model,omitempty"`

	// SessionEnd: Reason is why the session ended. A `claude -p` run ends with
	// reason "other" (verified against claude 2.1.282; see EVIDENCE.md).
	// sr:docs https://code.claude.com/docs/en/hooks#sessionend
	Reason string `json:"reason,omitempty"`

	// UserPromptSubmit
	// Prompt is the raw text of the user message being submitted. The real Claude
	// Code payload names this field "prompt" (NOT "user_prompt").
	// sr:docs https://code.claude.com/docs/en/hooks#userpromptsubmit
	Prompt string `json:"prompt,omitempty"`

	// Stop / SubagentStop. Real Claude Code sends stop_hook_active (false
	// included), last_assistant_message, background_tasks and session_crons on
	// both — pointers here so they are present exactly on those events.
	// sr:docs https://code.claude.com/docs/en/hooks#stop-input
	StopHookActive       *bool             `json:"stop_hook_active,omitempty"`
	LastAssistantMessage *string           `json:"last_assistant_message,omitempty"`
	BackgroundTasks      *[]BackgroundTask `json:"background_tasks,omitempty"`
	SessionCrons         *[]any            `json:"session_crons,omitempty"`

	// PreToolUse / PostToolUse
	ToolName string `json:"tool_name,omitempty"`
	// ToolUseID is the id of the tool_use block the event is about. Real Claude
	// Code sends it on PreToolUse and PostToolUse, and records it as the
	// toolUseID of the hook's attachment.
	// sr:docs https://code.claude.com/docs/en/hooks#pretooluse-input
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	// ToolResponse is PostToolUse's tool_response: the tool's structured
	// result where it has one (Bash: {stdout, stderr, interrupted, …}), else
	// the text the model got.
	// sr:docs https://code.claude.com/docs/en/hooks#posttooluse-input
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
	// PostToolUseFailure: error is the text the model got, is_interrupt
	// whether the user interrupted it. Both carry duration_ms, the tool's run
	// time, as PostToolUse does (claude 2.1.282 payloads).
	Error       string `json:"error,omitempty"`
	IsInterrupt *bool  `json:"is_interrupt,omitempty"`
	DurationMs  *int64 `json:"duration_ms,omitempty"`

	// SubagentStart / SubagentStop — agent type name (matcher for these events)
	AgentType string `json:"agent_type,omitempty"`
	// SubagentStart / SubagentStop — unique per-subagent id generated by the parent
	// when it spawns a subagent via the Agent (alias Task) tool. The real Claude
	// hook payload carries this so the SubagentStart and matching SubagentStop can
	// be correlated.
	// sr:docs https://code.claude.com/docs/en/hooks#subagentstart
	AgentID string `json:"agent_id,omitempty"`
	// SubagentStop — path to the subagent's OWN transcript file
	// (<session>/subagents/agent-<agent_id>.jsonl). The real Claude Code SubagentStop
	// payload carries this documented field so hooks can read the dispatch prompt
	// (with --task-id) directly without deriving the path from transcript_path.
	// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
	AgentTranscriptPath string `json:"agent_transcript_path,omitempty"`

	// WorktreeCreate / WorktreeRemove
	WorktreeName string `json:"worktree_name,omitempty"`

	// PreCompact / PostCompact: what triggered the compaction ("manual" |
	// "auto"); PreCompact's custom_instructions (null unless a manual /compact
	// passed some), PostCompact's compact_summary.
	// sr:docs https://code.claude.com/docs/en/hooks#precompact-input
	Trigger            string          `json:"trigger,omitempty"`
	CustomInstructions json.RawMessage `json:"custom_instructions,omitempty"`
	CompactSummary     *string         `json:"compact_summary,omitempty"`
}

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

// MarshalJSON writes the payload, with agent_type present (possibly "")
// whenever agent_id is: real Claude Code sends both together — a manual
// compaction's summarizer fires SubagentStop with agent_type "" (claude
// 2.1.282).
func (in Input) MarshalJSON() ([]byte, error) {
	type plain Input
	b, err := json.Marshal(plain(in))
	if err != nil || in.AgentID == "" || in.AgentType != "" {
		return b, err
	}
	return append(b[:len(b)-1], []byte(`,"agent_type":""}`)...), nil
}
