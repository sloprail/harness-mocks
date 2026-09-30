package hooks

import (
	"strings"
)

// BlockError is a handler that exited 2: its command and its stderr exactly as
// it wrote it, which real Claude Code quotes as "[<command>]: <stderr>".
type BlockError struct {
	Command string
	Stderr  string
}

func (e *BlockError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = "hook blocked the action"
	}
	return "hooks: command blocked: " + msg
}

// Quoted is the text real Claude Code shows for an exit-2 block:
// "[<command>]: <stderr>", or "No stderr output" in place of an empty stderr
// (claude 2.1.282, the hook runner's exit-2 branch).
func (e *BlockError) Quoted() string {
	return QuoteBlock(e.Command, e.Stderr)
}

// QuoteBlock renders an exit-2 handler the way real Claude Code quotes it.
func QuoteBlock(command, stderr string) string {
	if stderr == "" {
		stderr = "No stderr output"
	}
	return "[" + command + "]: " + stderr
}
