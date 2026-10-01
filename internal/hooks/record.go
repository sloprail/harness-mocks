package hooks

// Moment is when in a session a hook fires.
type Moment int

const (
	MomentSessionStart Moment = iota
	MomentSessionEnd
	MomentPrompt
	MomentPreTool
	MomentPostTool
	MomentToolFailed
	MomentStop
	MomentSubagentStart
	MomentSubagentStop
	MomentCompaction
	MomentWorktree
)

// Ran is what one hook command did, as far as a transcript record depends on.
type Ran struct {
	Moment Moment
	// Blocked: it blocked by exit status.
	Blocked bool
	// DecisionBlock: it exited 0 and blocked by what it printed.
	DecisionBlock bool
	// Deny: it refused a tool call by what it printed.
	Deny bool
	// Cancelled: its timeout stopped it.
	Cancelled bool
	// JSONError: what it printed looked like JSON and did not parse or validate.
	JSONError bool
	// FailedStatus: it exited non-zero without blocking, and nothing it
	// printed decided.
	FailedStatus bool
	// Printed: it wrote to stdout or stderr.
	Printed bool
	// Context: what it printed carried context for the agent.
	Context bool
}

// Attachment is the kind of record a hook run leaves.
type Attachment int

const (
	AttachNone Attachment = iota
	// AttachSuccess: its output.
	AttachSuccess
	// AttachNonBlockingError: an error that did not stop anything.
	AttachNonBlockingError
	// AttachBlockingError: a block.
	AttachBlockingError
	// AttachCancelled: its timeout stopped it.
	AttachCancelled
)

// Record is what a hook run leaves in the session's transcript.
type Record struct {
	Attachment Attachment
	// Feedback: the reason is also handed back to the agent as a message of
	// its own, because the block continues the turn.
	Feedback bool
	// Context: the context it carried is recorded too.
	Context bool
}

// LeavesRecords reports whether the hooks of a moment leave records at all.
// A session's end, a compaction and a worktree leave none: what their hooks
// print is shown to the user only.
func LeavesRecords(m Moment) bool {
	switch m {
	case MomentSessionEnd, MomentCompaction, MomentWorktree:
		return false
	}
	return true
}

// RecordFor is what a hook run leaves in the transcript: a silent success
// nothing, and printed output, an error, a block or a cancellation a record
// of their own. A block continues the turn when it is the end of a turn's, and
// is then also given back to the agent as feedback.
func RecordFor(r Ran) Record {
	if !LeavesRecords(r.Moment) {
		return Record{}
	}
	stopLike := r.Moment == MomentStop || r.Moment == MomentSubagentStop
	toolDone := r.Moment == MomentPostTool || r.Moment == MomentToolFailed
	switch {
	case r.Cancelled:
		return Record{Attachment: AttachCancelled}
	case r.Blocked:
		switch {
		case r.Moment == MomentSessionStart || r.Moment == MomentSubagentStart:
			return Record{Attachment: AttachNonBlockingError}
		case stopLike:
			return Record{Feedback: true}
		case toolDone:
			return Record{Attachment: AttachBlockingError}
		}
		return Record{}
	case r.DecisionBlock && (stopLike || r.Moment == MomentPostTool):
		return Record{Attachment: AttachBlockingError, Feedback: stopLike}
	case r.Moment == MomentPreTool && r.Deny:
		return Record{}
	case r.JSONError, r.FailedStatus:
		return Record{Attachment: AttachNonBlockingError}
	case r.Printed:
		// A prompt hook's context stands alone: it is the record.
		if r.Moment == MomentPrompt && r.Context {
			return Record{Context: true}
		}
		return Record{Attachment: AttachSuccess, Context: r.Context}
	}
	return Record{}
}

// SummaryAfter reports whether the hooks of a moment end with a summary record
// of their own: each end of a turn that ran any hook does, a sub-agent's does
// not.
func SummaryAfter(m Moment, handlers int) bool { return m == MomentStop && handlers > 0 }
