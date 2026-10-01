package hooks

import (
	"encoding/json"
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// commandRun is the HandlerRun of a command hook that has run: what it
// printed, read the way Claude Code reads it, and what its exit status makes
// of it. A command that timed out was cancelled and its output is discarded.
func commandRun(h HandlerSpec, ev EventName, o corehooks.Outcome) HandlerRun {
	run := HandlerRun{
		Command: h.Command, Stdout: o.Stdout, Stderr: o.Stderr, ExitCode: o.Exit, DurationMs: o.Took.Milliseconds(),
	}
	if o.TimedOut {
		run.TimedOut, run.TimeoutMs = true, o.Timeout.Milliseconds()
		return run
	}
	if !o.Started {
		run.ExitCode = 0
		return run
	}
	// Claude Code reads the JSON on every exit code, not only 0 (docs, "Exit
	// code output"): parse it before the status decides anything.
	if o.Stdout != "" {
		switch {
		case !corehooks.IsJSONOutput(o.Stdout, isOutputField):
			run.Output.PlainText = strings.TrimSpace(o.Stdout)
		case !json.Valid([]byte(o.Stdout)):
			run.JSONError = "Hook output looks like a JSON object but is not valid JSON"
		default:
			if msg := decodeOutput([]byte(o.Stdout), &run.Output); msg != "" {
				run.JSONError = msg
			} else {
				run.JSONParsed = true
			}
		}
	}
	// sr:provides hook-exit-code-semantics/claude
	if corehooks.VerdictOf(o.Exit, strictExitEvents[ev]) == corehooks.Blocked {
		run.Blocked = true
		if run.JSONParsed {
			run.BlockReason = jsonBlockReason(run.Output)
		}
	}
	return run
}

// blockError is the BlockError of a run that blocked.
func blockError(run HandlerRun) *BlockError {
	suppress := run.JSONParsed && run.Output.HookSpecificOutput != nil && run.Output.HookSpecificOutput.SuppressOriginalPrompt
	return &BlockError{Command: run.Command, Stderr: run.Stderr, Reason: run.BlockReason, SuppressPrompt: suppress}
}
