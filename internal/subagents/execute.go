package subagents

import "github.com/sloprail/harness-mocks/internal/tasks"

// Outcome is how one run of a sub-agent ended.
type Outcome struct {
	// FinalText is its final report, LastAssistant the text of its last message.
	FinalText, LastAssistant string
	// Failure is why it failed, "" when it did not.
	Failure  string
	ToolUses int
}

// Hooks are what a harness fires around a sub-agent.
type Hooks struct {
	// Start fires when the sub-agent starts. It cannot block the sub-agent, so it
	// returns nothing: the sub-agent runs whatever the hook did.
	Start func()
	// Stop fires when the sub-agent stops, with whether an earlier stop hook
	// blocked it already (active) and its last message; it reports whether it
	// blocked, and why.
	Stop func(active bool, last string) (blocked bool, reason string)
	// OnRerun and OnCap, when set, report a re-run after a block and a block cap
	// being reached.
	OnRerun func(reason string, turn int)
	OnCap   func(blockCap int)
}

// StopFacts is what a sub-agent's stop hook is told: the sub-agent's own
// transcript, its last message, and the session's background tasks (not only
// the sub-agent's).
type StopFacts struct {
	TranscriptPath, LastMessage string
	Tasks                       []*tasks.Task
}

// Stop is the facts of a sub-agent's stop for the hook: its own transcript
// path and last message, and every background task still running in the session.
func Stop(transcriptPath, last string, session *tasks.Registry) StopFacts {
	return StopFacts{TranscriptPath: transcriptPath, LastMessage: last, Tasks: session.Running()}
}

// Execute fires the start hook, runs the sub-agent, and re-runs it while the stop
// hook blocks: the hook's reason is its feedback, until the hook lets the
// sub-agent stop or blockCap consecutive blocks have been honoured (0: no cap).
// It returns how the last run ended, its tool uses summed over every run.
func Execute(h Hooks, blockCap int, run func() Outcome) Outcome {
	if h.Start != nil {
		h.Start()
	}
	out := run()
	for turn := 0; ; turn++ {
		blocked, reason := h.Stop(turn > 0, out.LastAssistant)
		if !blocked {
			return out
		}
		if blockCap > 0 && turn >= blockCap {
			if h.OnCap != nil {
				h.OnCap(blockCap)
			}
			return out
		}
		if h.OnRerun != nil {
			h.OnRerun(reason, turn+1)
		}
		next := run()
		next.ToolUses += out.ToolUses
		out = next
	}
}
