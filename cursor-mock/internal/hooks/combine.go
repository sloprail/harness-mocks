package hooks

import (
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

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

// Contexts are the additional context the event's hooks gave, as a tool call's
// frame carries it: one entry naming the event with all the hooks' texts, in the
// order the hooks are configured in, set apart by a rule; none when no hook gave
// any (recorded: runs/additional-context).
func Contexts(e Event, ds []Decision) []any {
	var texts []string
	for _, d := range ds {
		if d.Context != "" {
			texts = append(texts, d.Context)
		}
	}
	if len(texts) == 0 {
		return nil
	}
	return []any{map[string]any{"hookEventName": string(e), "content": strings.Join(texts, "\n\n---\n\n")}}
}
