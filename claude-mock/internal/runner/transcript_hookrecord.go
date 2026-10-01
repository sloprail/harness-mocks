package runner

import (
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// momentOf is the moment in a session a Claude Code hook event fires at. An
// event with no evidence of leaving records (a worktree's, a compaction's) maps
// to a moment that leaves none.
func momentOf(ev hooks.EventName) corehooks.Moment {
	switch ev {
	case hooks.EventSessionStart:
		return corehooks.MomentSessionStart
	case hooks.EventSessionEnd:
		return corehooks.MomentSessionEnd
	case hooks.EventUserPromptSubmit:
		return corehooks.MomentPrompt
	case hooks.EventPreToolUse:
		return corehooks.MomentPreTool
	case hooks.EventPostToolUse:
		return corehooks.MomentPostTool
	case hooks.EventPostToolUseFailure:
		return corehooks.MomentToolFailed
	case hooks.EventStop:
		return corehooks.MomentStop
	case hooks.EventSubagentStart:
		return corehooks.MomentSubagentStart
	case hooks.EventSubagentStop:
		return corehooks.MomentSubagentStop
	case hooks.EventPreCompact, hooks.EventPostCompact:
		return corehooks.MomentCompaction
	}
	return corehooks.MomentWorktree
}

// ranOf is what one handler did, in the terms core's RecordFor decides on.
func ranOf(ev hooks.EventName, r hooks.HandlerRun, context string) corehooks.Ran {
	return corehooks.Ran{
		Moment: momentOf(ev), Blocked: r.Blocked, DecisionBlock: !r.Blocked && r.Output.Decision == "block",
		Deny: isDeny(r.Output), Cancelled: r.TimedOut, JSONError: r.JSONError != "" || r.HTTPError != "",
		FailedStatus: r.ExitCode != 0 && !r.JSONParsed, Printed: r.Stdout != "" || r.Stderr != "", Context: context != "",
	}
}

// nonBlockingMessage is the stderr of a non-blocking error's record: the JSON
// parse or validation message (and, on a non-zero exit, the hook's own stderr
// after it; recorded: snapshots/runs/hook-exit-json), an HTTP hook's failure,
// or "Failed with non-blocking status code: ..." for a bare non-zero status.
func nonBlockingMessage(r hooks.HandlerRun) string {
	switch {
	case r.HTTPError != "":
		return r.HTTPError
	case r.JSONError != "":
		msg := r.JSONError
		if r.ExitCode != 0 {
			msg += fmt.Sprintf("\n\nHook exited %d with stderr:\n%s", r.ExitCode, strings.TrimSpace(r.Stderr))
		}
		return msg
	}
	msg := strings.TrimSpace(r.Stderr)
	if msg == "" {
		msg = "No stderr output"
	}
	return "Failed with non-blocking status code: " + msg
}

// recordExit2 writes what a handler that exited 2 leaves, as core decided it:
// a non-blocking error for a start event, the feedback meta turn for the end
// of a turn, a blocking error after a tool. The tool the hook ran before, a
// prompt and the rest leave nothing here (their refusal is shown elsewhere).
func (t *transcript) recordExit2(att func(string, map[string]any), s *stopSummary, ev hooks.EventName, r hooks.HandlerRun, rec corehooks.Record) {
	quoted := hooks.QuoteRun(r)
	if rec.Feedback {
		t.stopHookFeedback(quoted)
		s.errors = append(s.errors, quoted)
	}
	switch rec.Attachment {
	case corehooks.AttachNonBlockingError:
		att("hook_non_blocking_error", map[string]any{
			"stderr": quoted, "stdout": r.Stdout, "exitCode": r.ExitCode, "command": r.Command,
		})
	case corehooks.AttachBlockingError:
		att("hook_blocking_error", map[string]any{
			"blockingError": map[string]any{"blockingError": quoted, "command": r.Command},
		})
	}
	s.hasOutput = true
}
