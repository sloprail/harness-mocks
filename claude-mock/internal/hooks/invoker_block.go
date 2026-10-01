package hooks

import (
	"strings"
)

// BlockError is a handler that exited 2: its command and its stderr exactly as
// it wrote it, which real Claude Code quotes as "[<command>]: <stderr>".
//
// Reason is the reason the hook's JSON gave when it also made a blocking
// decision: Claude Code then shows that, not the stderr (recorded:
// snapshots/runs/hook-exit-json).
type BlockError struct {
	Command string
	Stderr  string
	Reason  string
	// SuppressPrompt: the hook's JSON asked for the blocked prompt to be left
	// out of the block message.
	SuppressPrompt bool
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
	if e.Reason != "" {
		return e.Reason
	}
	return QuoteBlock(e.Command, e.Stderr)
}

// QuoteRun is how a blocked handler's message reads: its JSON's blocking
// reason when it gave one, else its quoted stderr.
func QuoteRun(r HandlerRun) string {
	if r.BlockReason != "" {
		return r.BlockReason
	}
	return QuoteBlock(r.Command, r.Stderr)
}

// jsonBlockReason is the reason of a blocking decision in Claude Code's hook
// JSON (decision "block", or a PreToolUse permissionDecision "deny"), if any.
func jsonBlockReason(o Output) string {
	if o.Decision == "block" && o.Reason != "" {
		return o.Reason
	}
	if h := o.HookSpecificOutput; h != nil && h.PermissionDecision == "deny" {
		return h.PermissionDecisionReason
	}
	return ""
}

// QuoteBlock renders an exit-2 handler the way real Claude Code quotes it.
func QuoteBlock(command, stderr string) string {
	if stderr == "" {
		stderr = "No stderr output"
	}
	return "[" + command + "]: " + stderr
}
