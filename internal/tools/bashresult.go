package tools

import (
	"fmt"
	"strings"
)

// Message is what the agent is told of a shell command that ran in the
// foreground: its output with the trailing newlines trimmed, and isError
// false; for a command that ended with a non-zero status, an error that
// states the exit code and then the output.
func (r BashResult) Message() (text string, isError bool) {
	out := strings.TrimRight(r.Output, "\n")
	if !r.Failed() {
		return out, false
	}
	msg := fmt.Sprintf("Exit code %d", r.ExitCode)
	if out != "" {
		msg += "\n" + out
	}
	return msg, true
}
