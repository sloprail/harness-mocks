package hooks

import (
	"encoding/json"
	"fmt"
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// Decision is what one hook command decided about the action it was asked
// about.
type Decision struct {
	// Permission is "deny", "ask" or "allow", or "" when the hook decided
	// nothing.
	Permission string
	// Blocked: the hook blocked by its exit status, not by its output.
	Blocked bool
	// Message is what a refusal says: the hook's stderr after "Hook blocked
	// with message:", the user_message of a deny, or the invalid-JSON notice.
	Message string
	// Context is the additional_context the hook gave, for an event that takes
	// it (sessionStart, postToolUse, postToolUseFailure).
	Context string
}

// failClosedNote opens what a fail-closed hook's failure says.
const failClosedNote = "Tool blocked because this hook is configured to fail closed (block when it fails). "

// Interpret reads one hook command's outcome for the event. Its exit status
// classifies it (the core's VerdictOf): 2 blocks, which is a deny; any other
// non-zero status fails open, and the hook's output is not read. A hook
// configured failClosed blocks on any failure instead: a non-zero status, or
// no output from a permission hook. On a permission event, output that is not
// a JSON object blocks the action for safety (a hook that prints nothing
// allows it); otherwise its permission and user_message are read.
//
// sr:provides hook-exit-code-semantics/cursor
// sr:docs https://cursor.com/docs/hooks#command-based-hooks
func Interpret(e Event, h Entry, o corehooks.Outcome) Decision {
	if !o.Counts() {
		// The core says a result is not read when the hook timed out (it and its
		// children were killed, and whatever it printed is discarded) or could not
		// start: the action goes on. Cursor's one difference: a hook that timed
		// out and fails closed blocks, saying it timed out after the limit it ran
		// under.
		// sr:provides hook-timeout/cursor
		// sr:docs https://cursor.com/docs/hooks#per-script-configuration-options
		if corehooks.FailsClosed(o, h.FailClosed) {
			return Decision{Permission: "deny", Blocked: true, Message: fmt.Sprintf("%sHook %q execution failed: Hook script timed out after %dms", failClosedNote, h.Command, o.Timeout.Milliseconds())}
		}
		return Decision{}
	}
	verdict := corehooks.NonBlockingError
	if o.Exit >= 0 {
		verdict = corehooks.VerdictOf(o.Exit, h.FailClosed)
	}
	switch verdict {
	case corehooks.Blocked:
		if o.Exit != 2 {
			return Decision{Permission: "deny", Blocked: true, Message: fmt.Sprintf("%sHook %q failed with exit code %d: %s", failClosedNote, h.Command, o.Exit, strings.TrimSpace(o.Stderr))}
		}
		return Decision{Permission: "deny", Blocked: true, Message: "Hook blocked with message: " + strings.TrimSpace(o.Stderr)}
	case corehooks.NonBlockingError:
		return Decision{}
	}
	out := strings.TrimSpace(o.Stdout)
	if e.addsContext() {
		// sr:provides hook-additional-context/cursor
		// sr:docs https://cursor.com/docs/hooks#posttooluse
		var p struct {
			Context string `json:"additional_context"`
		}
		if corehooks.IsJSONOutput(out, isOutputField) && json.Unmarshal([]byte(out), &p) == nil {
			return Decision{Context: p.Context}
		}
		return Decision{}
	}
	if !e.permission() {
		return Decision{}
	}
	if out == "" {
		if corehooks.SilentFails(out, h.FailClosed) {
			return Decision{Permission: "deny", Message: fmt.Sprintf("%sHook %q returned no output.", failClosedNote, h.Command)}
		}
		return Decision{}
	}
	var p struct {
		Permission  string `json:"permission"`
		UserMessage string `json:"user_message"`
	}
	// What counts as JSON output is the core's; the fields read from it are Cursor's.
	if !corehooks.IsJSONOutput(out, isOutputField) || json.Unmarshal([]byte(out), &p) != nil {
		return Decision{Permission: "deny", Message: fmt.Sprintf("Hook %q returned invalid JSON. The command was blocked for safety.", h.Command)}
	}
	// JSON whose permission is not one of Cursor's is worded differently
	// (recorded: runs/before-read-refusal, {"permission":"maybe"})
	if !validPermission(p.Permission) {
		if e == BeforeReadFile {
			return Decision{Permission: "deny", Message: fmt.Sprintf("Hook %q returned an invalid response for this hook step. The command was blocked for safety.", h.Command)}
		}
		// no recording shows how the other permission events word it
		return Decision{Permission: "deny", Message: fmt.Sprintf("Hook %q returned invalid JSON. The command was blocked for safety.", h.Command)}
	}
	return Decision{Permission: p.Permission, Message: p.UserMessage}
}

func validPermission(p string) bool { return p == "" || p == "allow" || p == "deny" || p == "ask" }

// isOutputField reports whether a key is one a Cursor hook's output sets.
func isOutputField(key string) bool {
	switch key {
	case "permission", "user_message", "agent_message", "continue", "env", "additional_context":
		return true
	}
	return false
}

// refusalSeparator joins the messages of the hooks that refused one call
// (recorded: runs/pretool-refusal-combined).
const refusalSeparator = "\n\n---\n\n"

// Refusal is whether several hooks' decisions refuse the call, and what the
// refusal says: the core decides (one refusing hook refuses it, whatever the
// others decided), and Cursor's parameter of it is that the message is the
// messages of every hook that refused, a block by exit status and a deny in
// output alike, in the order the hooks are configured in, with the separator
// between them. An "ask" decides nothing: the mock models a run that no one is
// there to ask.
//
// sr:provides pretooluse-refusal/cursor
// sr:docs https://cursor.com/docs/hooks#configuration
func Refusal(ds []Decision) (refused bool, message string) {
	votes := make([]corehooks.PreToolVote, len(ds))
	for i, d := range ds {
		votes[i] = corehooks.PreToolVote{Blocked: d.Blocked, BlockReason: d.Message, Denied: d.Permission == "deny", DenyReason: d.Message}
	}
	return corehooks.PreToolRefusal(votes, refusalSeparator)
}
