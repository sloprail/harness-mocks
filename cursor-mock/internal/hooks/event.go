// Package hooks is Cursor's hook surface: the hooks.json it reads, the events
// it fires with their payloads, and how a hook's result is read. What a
// verdict or a refusal means is decided in the shared core, internal/hooks.
package hooks

// Event is a Cursor hook event.
type Event string

// The events the mock fires. Cursor's docs name more (beforeSubmitPrompt, stop,
// afterAgentResponse, subagentStart/Stop, the MCP and Tab hooks, preCompact,
// workspaceOpen); the recordings show cursor-agent in print
// mode firing none of the first three, and the mock does not model the rest.
const (
	SessionStart         Event = "sessionStart"
	SessionEnd           Event = "sessionEnd"
	PreToolUse           Event = "preToolUse"
	PostToolUse          Event = "postToolUse"
	PostToolUseFailure   Event = "postToolUseFailure"
	BeforeShellExecution Event = "beforeShellExecution"
	AfterShellExecution  Event = "afterShellExecution"
	AfterFileEdit        Event = "afterFileEdit"
	BeforeReadFile       Event = "beforeReadFile"
)

// addsContext reports whether a hook of the event can hand the agent context,
// as the additional_context of its JSON output: at the start of the session,
// and after a tool call, whether it succeeded or failed.
//
// sr:docs https://cursor.com/docs/hooks#sessionstart
func (e Event) addsContext() bool {
	return e == SessionStart || e == PostToolUse || e == PostToolUseFailure
}

// permission reports whether a hook of the event answers with a permission
// decision: for these, output that is not a JSON object blocks the action.
//
// sr:docs https://cursor.com/docs/hooks#command-based-hooks
func (e Event) permission() bool {
	return e == PreToolUse || e == BeforeShellExecution
}
