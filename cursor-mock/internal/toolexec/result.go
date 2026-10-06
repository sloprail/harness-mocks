package toolexec

import (
	"strings"
	"time"
)

// NotModeledPrefix starts the message of a call the mock refuses because no
// recording shows what the harness does with it.
const NotModeledPrefix = "cursor-mock: "

// NotModeled is the message of a failed call that is a refusal of something the
// mock does not model, and whether it is one.
func (r Result) NotModeled() (string, bool) {
	return r.ErrorMessage, r.Failed && strings.HasPrefix(r.ErrorMessage, NotModeledPrefix)
}

// Result is what a tool call came to.
type Result struct {
	// Failed: the call ran and failed.
	Failed bool
	// Frame is the result object of the call's completed stream frame.
	Frame map[string]any
	// ToolOutput is the call's result as postToolUse reports it, a JSON string.
	ToolOutput string
	// ErrorMessage is what postToolUseFailure reports when the call failed.
	ErrorMessage string
	// Output is what a shell command printed, for afterShellExecution.
	Output string
	// Background: the call started a background shell (no afterShellExecution
	// follows it; its end comes as a task notification).
	Background bool
	// Read is what a successful read returned, for beforeReadFile.
	Read *ReadFile
	// Edits are the changes a file write made, for afterFileEdit.
	Edits []Edit
	Took  time.Duration
}

// ReadFile is the file a read returned.
type ReadFile struct{ Path, Content string }
