package hooks

import (
	"encoding/json"
	"fmt"
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// Decision is what one hook command's outcome decides on an event.
type Decision struct {
	// Blocked: the command exited 2 (BlockReason is its stderr). A block by
	// status is final: the command's output is not read.
	Blocked     bool
	BlockReason string
	// Denied: the command's JSON refused (a deny decision or `decision:
	// "block"`), with DenyReason.
	Denied     bool
	DenyReason string
	// Permission is a before-tool decision the command accepted or denied
	// ("allow", "deny"), for ranking when several hooks decide one call.
	Permission string
	// Halt: a Stop hook printed `continue: false`, which ends the turn whatever
	// the other matching Stop hooks decided; a SessionStart hook's, which ends
	// the turn before it begins, the session having started all the same
	// (recorded: runs/session-start-continue-false).
	Halt bool
	// Context is text the hook adds as developer context.
	Context string
	// Error is set when the hook failed without blocking anything: it could
	// not start, timed out, exited with a status other than 0 and 2, or printed
	// output the event does not take.
	Error string
}

// Interpret reads one hook command's outcome for an event. Exit status 2 blocks
// with the command's stderr as the reason, whatever it printed; any other
// non-zero status, a command that cannot start or one that times out is a
// non-blocking error, and its output is not read; exit 0 reads the output.
//
// sr:provides hook-exit-code-semantics/codex
// sr:docs https://developers.openai.com/codex/hooks#pretooluse
func Interpret(ev Event, o corehooks.Outcome) Decision {
	switch {
	case !o.Started:
		return Decision{Error: "hook command could not be started: " + o.Command}
	case o.TimedOut:
		return Decision{Error: "hook timed out: " + o.Command}
	}
	switch corehooks.VerdictOf(o.Exit, false) {
	case corehooks.Blocked:
		return Decision{Blocked: true, BlockReason: strings.TrimSpace(o.Stderr)}
	case corehooks.NonBlockingError:
		return Decision{Error: fmt.Sprintf("hook exited with code %d: %s", o.Exit, o.Command)}
	}
	return readOutput(ev, strings.TrimSpace(o.Stdout))
}

type output struct {
	Decision *string `json:"decision"`
	Reason   string  `json:"reason"`
	Continue *bool   `json:"continue"`
	Specific *struct {
		Permission string `json:"permissionDecision"`
		Reason     string `json:"permissionDecisionReason"`
		Context    string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func readOutput(ev Event, s string) Decision {
	if s == "" {
		return Decision{}
	}
	if !strings.HasPrefix(s, "{") { // plain text
		switch ev {
		case SessionStart, UserPromptSubmit:
			return Decision{Context: s}
		case Stop:
			return Decision{Error: "plain text output is invalid for Stop"}
		}
		return Decision{}
	}
	var out output
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return Decision{Error: "hook output is not valid JSON for " + string(ev) + ": " + err.Error()}
	}
	d := Decision{}
	if out.Specific != nil {
		d.Context = out.Specific.Context
	}
	if out.Decision != nil && *out.Decision == "block" {
		d.Denied, d.DenyReason, d.Permission = true, out.Reason, "deny"
	}
	if (ev == Stop || ev == SessionStart || ev == SubagentStop) && out.Continue != nil && !*out.Continue {
		d.Halt = true
	}
	if ev == PreToolUse {
		return preToolOutput(d, out)
	}
	return d
}

// preToolOutput reads a before-tool hook's decision: deny refuses, allow
// accepts; ask, defer, approve and `continue: false` are parsed but
// unsupported, and fail the hook instead.
func preToolOutput(d Decision, out output) Decision {
	if out.Continue != nil || (out.Decision != nil && *out.Decision != "block") {
		return Decision{Error: "unsupported PreToolUse output field"}
	}
	if out.Specific == nil {
		return d
	}
	switch out.Specific.Permission {
	case "deny":
		d.Denied, d.DenyReason, d.Permission = true, out.Specific.Reason, "deny"
	case "allow":
		d.Permission = corehooks.StrongerPermission(d.Permission, "allow")
	case "":
	default:
		return Decision{Error: "unsupported PreToolUse permissionDecision: " + out.Specific.Permission}
	}
	return d
}
