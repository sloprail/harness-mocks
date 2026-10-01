package hooks

import "encoding/json"

// ToolOutcome is how a tool call ended, as the hooks after it see it.
type ToolOutcome int

const (
	// ToolRefused: a before-tool hook refused the call; it never ran.
	ToolRefused ToolOutcome = iota
	// ToolSucceeded: it ran and succeeded.
	ToolSucceeded
	// ToolFailed: it ran and failed (a command exiting non-zero).
	ToolFailed
	// ToolErrored: it ended in an error without running as asked (a tool the
	// harness does not have).
	ToolErrored
)

// AfterTool is which hook fires after a tool call.
type AfterTool int

const (
	// AfterNone: no hook fires.
	AfterNone AfterTool = iota
	// AfterSuccess: the success hook, with the tool's response.
	AfterSuccess
	// AfterFailure: the failure hook, instead of the success hook, with the
	// error text the agent received.
	AfterFailure
)

// RejectedInput is the required parameters a call's input lacks, in the
// order given. A call with any is rejected before execution: no hook fires
// for it, the before-tool hooks included, and the tool does not run; the
// harness answers it with its validation error.
func RejectedInput(input json.RawMessage, required []string) []string {
	var got map[string]json.RawMessage
	_ = json.Unmarshal(input, &got)
	var missing []string
	for _, p := range required {
		if _, ok := got[p]; !ok {
			missing = append(missing, p)
		}
	}
	return missing
}

// AfterToolHook is the hook a tool call's outcome fires: a call that ran fires
// the success or the failure hook, never both; a refused or an errored call
// fires neither (and a call whose input was rejected reached no hook at all:
// RejectedInput).
//
// sr:capability tool-failure-hook
func AfterToolHook(o ToolOutcome) AfterTool {
	switch o {
	case ToolSucceeded:
		return AfterSuccess
	case ToolFailed:
		return AfterFailure
	default:
		return AfterNone
	}
}
