package hooks

import corehooks "github.com/sloprail/harness-mocks/internal/hooks"

// Turn is the prompt a session is on (the core's).
type Turn = corehooks.Turn

// turnEvents are the events about a turn of the conversation: they carry
// permission_mode. A session's or sub-agent's start and end and a compaction do
// not (recorded: snapshots/runs/bgagent, compact).
var turnEvents = map[EventName]bool{
	EventUserPromptSubmit: true, EventPreToolUse: true, EventPostToolUse: true,
	EventPostToolUseFailure: true, EventStop: true, EventSubagentStop: true,
}

// promptEvent is how the event relates to the prompt the session is on: the
// user's prompt begins one, a task notification submitted as a prompt does not.
func promptEvent(in Input) corehooks.PromptEvent {
	switch {
	case in.HookEventName == EventUserPromptSubmit && !in.ContinuesPrompt:
		return corehooks.PromptBegins
	case turnEvents[in.HookEventName]:
		return corehooks.PromptContinues
	}
	return corehooks.PromptSurrounds
}
