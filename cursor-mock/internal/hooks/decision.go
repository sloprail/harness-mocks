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
}

// failClosedNote opens what a fail-closed hook's failure says.
const failClosedNote = "Tool blocked because this hook is configured to fail closed (block when it fails). "

// Interpret reads one hook command's result for the event. Its exit status
// classifies it (the core's VerdictOf, run by Invoke): 2 blocks, which is a
// deny; any other non-zero status fails open, and the hook's output is not
// read. A hook configured failClosed blocks on any failure instead: a
// non-zero status, or no output from a permission hook. On a permission
// event, output that is not a JSON object blocks the action for safety (a
// hook that prints nothing allows it); otherwise its permission and
// user_message are read.
//
// sr:provides hook-exit-code-semantics/cursor
// sr:docs https://cursor.com/docs/hooks#command-based-hooks
func Interpret(e Event, h Entry, run corehooks.Run) Decision {
	command := h.Command
	switch run.Verdict {
	case corehooks.Blocked:
		if run.ExitCode != 2 {
			return Decision{Permission: "deny", Blocked: true, Message: fmt.Sprintf("%sHook %q failed with exit code %d: %s", failClosedNote, command, run.ExitCode, strings.TrimSpace(run.Stderr))}
		}
		return Decision{Permission: "deny", Blocked: true, Message: "Hook blocked with message: " + strings.TrimSpace(run.Stderr)}
	case corehooks.NonBlockingError:
		return Decision{}
	}
	out := strings.TrimSpace(run.Stdout)
	if !e.permission() {
		return Decision{}
	}
	if out == "" {
		if h.FailClosed {
			return Decision{Permission: "deny", Message: fmt.Sprintf("%sHook %q returned no output.", failClosedNote, command)}
		}
		return Decision{}
	}
	var o struct {
		Permission  string `json:"permission"`
		UserMessage string `json:"user_message"`
	}
	if err := json.Unmarshal([]byte(out), &o); err != nil || !strings.HasPrefix(out, "{") || !validPermission(o.Permission) {
		return Decision{Permission: "deny", Message: fmt.Sprintf("Hook %q returned invalid JSON. The command was blocked for safety.", command)}
	}
	return Decision{Permission: o.Permission, Message: o.UserMessage}
}

func validPermission(p string) bool { return p == "" || p == "allow" || p == "deny" || p == "ask" }

// Decide is what several hooks decided about one call: it is refused when any
// hook denies it (a deny wins over ask, and ask over allow, whatever the
// order or the source), with the messages of the hooks that denied it
// concatenated; a block by exit status outranks a deny in output. An "ask"
// decides nothing: the mock models a run that no one is there to ask.
//
// sr:provides pretooluse-refusal/cursor
// sr:docs https://cursor.com/docs/hooks#configuration
func Decide(ds []Decision) (refused bool, message string) {
	perm := ""
	var blockMsgs, denyMsgs []string
	for _, d := range ds {
		perm = corehooks.StrongerPermission(perm, d.Permission)
		switch {
		case d.Blocked:
			blockMsgs = append(blockMsgs, d.Message)
		case d.Permission == "deny":
			denyMsgs = append(denyMsgs, d.Message)
		}
	}
	return corehooks.PreToolDecision(len(blockMsgs) > 0, strings.Join(blockMsgs, "\n"), perm == "deny", strings.Join(denyMsgs, "\n"))
}
