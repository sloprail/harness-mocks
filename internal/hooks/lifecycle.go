package hooks

// StartKind is how a session began.
type StartKind int

const (
	// StartFresh: a new session.
	StartFresh StartKind = iota
	// StartResumed: an existing session picked up again.
	StartResumed
	// StartForked: a new session branched from an existing one.
	StartForked
	// StartCompacted: the same session continuing after a compaction.
	StartCompacted
)

// SessionStartKind is how a session began. A compaction continues the session
// it compacted; a fork is a resume that got its own session, so it is a fork
// and not a resume.
//
// sr:capability session-start-hook
func SessionStartKind(resumed, forked, compacted bool) StartKind {
	switch {
	case compacted:
		return StartCompacted
	case forked:
		return StartForked
	case resumed:
		return StartResumed
	default:
		return StartFresh
	}
}

// BlocksSessionStart reports whether a hook's verdict stops the session from
// starting. None does: the session starts whatever its start hooks say.
func BlocksSessionStart(Verdict) bool { return false }

// StartHookEndsTurn reports whether a session-start hook that said it wants no
// more ("continue: false") ends the first turn: the session has started all
// the same, but no prompt hook fires and the model is not asked. Whether a
// harness honours it is the harness's: honoured.
func StartHookEndsTurn(saidStop, honoured bool) bool { return saidStop && honoured }

// EndReason is why a session ended.
type EndReason int

const (
	// EndOther: the generic reason of a session that ended on its own.
	EndOther EndReason = iota
	// EndInteractive: an ending only an interactive session has (a clear, a
	// switch to another session, a logout, an exit from the prompt).
	EndInteractive
)

// SessionEndReason is why a session ended. A non-interactive run has no
// interactive ending, so whatever the caller thinks happened, it ended for the
// generic reason.
//
// sr:capability session-end-hook
func SessionEndReason(interactive bool, why EndReason) EndReason {
	if !interactive {
		return EndOther
	}
	return why
}

// SessionEndRecordsOutput reports whether what a session-end hook prints is
// recorded in the session's transcript. It is not: the session has ended.
func SessionEndRecordsOutput() bool { return false }

// PromptSource is where a prompt handed to the agent came from.
type PromptSource int

const (
	// PromptFromUser: submitted by the user.
	PromptFromUser PromptSource = iota
	// PromptTaskNotification: a finished background task's notification,
	// handed to the agent as the next prompt.
	PromptTaskNotification
	// PromptSubagentDispatch: the prompt a sub-agent was dispatched with.
	PromptSubagentDispatch
)

// PromptHookFires reports whether the prompt hooks fire for a prompt: for what
// the user submits and for a background task's notification, not for the
// prompt a parent dispatched a sub-agent with.
func PromptHookFires(src PromptSource) bool { return src != PromptSubagentDispatch }

// PromptOutcome is what the prompt hooks decide: the prompt is refused when a
// hook blocked it, and otherwise goes to the agent unchanged with the context
// the hooks added. A hook never rewrites the prompt: there is no prompt in,
// and none out.
//
// sr:capability user-prompt-submit-hook
func PromptOutcome(blocked bool, context string) (refused bool, extra string) {
	if blocked {
		return true, ""
	}
	return false, context
}

// ContextOf is the text a hook adds to the agent's context: the context its
// structured output gave and, for an event that reads plain output as
// context, its plain output too (hooks that gave one each add both).
func ContextOf(structured, plain string, plainAdds bool) string {
	if !plainAdds {
		return structured
	}
	return JoinContext(structured, plain)
}

// JoinContext adds a hook's context to what earlier hooks of the run added:
// each adds, none replaces.
func JoinContext(have, add string) string {
	if have == "" || add == "" {
		return have + add
	}
	return have + "\n" + add
}

// ContextDue is whether the context a hook added reaches the agent now. A hook the agent
// waited for adds it at once. One that ran in the background (the agent did not wait for it) adds
// it at the next safe point: once the agent has gone through the call after the one it had
// begun when the hook started (startedAt calls begun then, steps now), or when its turn would
// end. The sync path asks it with the agent's step as it is at the hook (nothing is yet
// due for a background hook), the background path (Later.Due) as the agent goes on.
//
// sr:capability hook-additional-context
func ContextDue(background bool, startedAt, steps int, endOfTurn bool) bool {
	return !background || endOfTurn || startedAt < steps
}
