// Package toolcall is the harness-neutral order of one tool call: validate,
// ask the before-tool hooks, run, tell the after-tool hooks.
package toolcall

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/hooks"
)

// Call is a tool call the agent made.
type Call struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// Result is what a tool that ran produced.
type Result struct {
	Output string
	// Failed: it ran and failed (a command exiting non-zero).
	Failed bool
}

// Kind is how a call ended, which decides what the agent is told.
type Kind int

const (
	// Unknown: the harness has no such tool.
	Unknown Kind = iota
	// Invalid: the input lacks required parameters (Answer.Missing).
	Invalid
	// Refused: a before-tool hook refused it (Answer.Reason); it did not run.
	Refused
	// Done: it ran (Answer.Result), perhaps with its result replaced
	// (Answer.Feedback).
	Done
)

// Answer is the end of a call, handed to the host to tell the agent in its
// own words.
type Answer struct {
	Kind     Kind
	Missing  []string
	Reason   string
	Result   Result
	Feedback string
	// Replaced: an after-tool hook gave Feedback in place of the result.
	Replaced bool
}

// Host is the harness side of a tool call: its tools, its hooks, and how it
// words what the agent is told.
type Host interface {
	// Tool is the parameters a call to the named tool must carry, and whether
	// the harness has the tool.
	Tool(name string) (required []string, known bool)
	// Before asks the before-tool hooks whether they refuse the call.
	Before(ctx context.Context, c Call) (refused bool, reason string)
	// Execute runs the call.
	Execute(ctx context.Context, c Call) Result
	// After tells the hooks of the given kind about a call that ran, and
	// returns the feedback of one that blocks.
	After(ctx context.Context, c Call, r Result, kind hooks.AfterTool) (feedback string, blocked bool)
	// Answer tells the agent how the call ended; Run calls it exactly once.
	Answer(c Call, a Answer)
}

// LateInputCheck is implemented by a host whose harness checks some tools'
// input only after the before-tool hooks have run: the hooks then see a call
// that lacks required parameters, which is refused as invalid afterwards
// (and fires no after-tool hook), instead of being refused before any hook.
type LateInputCheck interface {
	// InputCheckedLate says whether the named tool's input is checked after
	// the before-tool hooks.
	InputCheckedLate(name string) bool
}

// Options are the parts of the order that differ between harnesses.
type Options struct {
	// SeparateFailureHook: the harness has a hook for a call that failed, which
	// fires instead of the success hook; without one, the success hook fires
	// for a failed call too.
	SeparateFailureHook bool
	// FailureOnRefusal: a call a before-tool hook refused fires the failure
	// hook (SeparateFailureHook), reporting the refusal as a failed call;
	// without it, a refused call fires no after-tool hook.
	FailureOnRefusal bool
}

// Run carries out one call. A call to a tool the harness lacks, and one whose
// input lacks required parameters, end before any hook. A call a before-tool
// hook refuses does not run and fires no after-tool hook (but the failure
// hook, with Options.FailureOnRefusal). A call that ran
// fires the after-tool hook its outcome calls for; a hook that blocks it
// replaces the result the agent sees, and the call has run.
func Run(ctx context.Context, h Host, c Call, o Options) {
	required, known := h.Tool(c.Name)
	if !known {
		h.Answer(c, Answer{Kind: Unknown})
		return
	}
	missing := hooks.RejectedInput(c.Input, required)
	late := false
	if l, ok := h.(LateInputCheck); ok {
		late = l.InputCheckedLate(c.Name)
	}
	if len(missing) > 0 && !late {
		h.Answer(c, Answer{Kind: Invalid, Missing: missing})
		return
	}
	if refused, reason := h.Before(ctx, c); refused {
		if o.FailureOnRefusal && o.SeparateFailureHook {
			h.After(ctx, c, Result{Output: reason, Failed: true}, hooks.AfterFailure)
		}
		h.Answer(c, Answer{Kind: Refused, Reason: reason})
		return
	}
	if len(missing) > 0 { // checked late: the hooks have seen the call
		h.Answer(c, Answer{Kind: Invalid, Missing: missing})
		return
	}
	r := h.Execute(ctx, c)
	outcome := hooks.ToolSucceeded
	if r.Failed {
		outcome = hooks.ToolFailed
	}
	kind := hooks.AfterToolHook(outcome)
	if kind == hooks.AfterFailure && !o.SeparateFailureHook {
		kind = hooks.AfterSuccess
	}
	feedback, blocked := h.After(ctx, c, r, kind)
	h.Answer(c, Answer{Kind: Done, Result: r, Feedback: feedback, Replaced: blocked})
}
