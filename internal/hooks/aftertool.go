package hooks

// ToolOutcome is how a tool call ended, as the hooks after it see it.
type ToolOutcome int

const (
	// ToolRefused: a before-tool hook refused the call; it never ran.
	ToolRefused ToolOutcome = iota
	// ToolSucceeded: it ran and succeeded.
	ToolSucceeded
	// ToolFailed: it ran and failed (a command exiting non-zero).
	ToolFailed
	// ToolErrored: it ended in an error without running as asked (invalid
	// input, a file tool's error).
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

// AfterToolHook is the hook a tool call's outcome fires: a call that ran fires
// the success or the failure hook, never both; a refused call fires neither.
// An errored call fires neither as well: no harness was seen firing one.
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
