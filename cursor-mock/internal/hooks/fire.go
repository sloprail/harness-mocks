package hooks

import (
	"context"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// Hooks fires a session's hooks.
type Hooks struct {
	Config Config
	// Dir is where project hooks run: the project root.
	Dir string
	// Env is the environment a hook command starts with.
	Env []string
	// Common builds the fields every payload carries, at the time of the event.
	Common func() Common
}

// Fire runs every command configured for the event, in order, with the
// event's payload on stdin, and returns what each decided. A project hook runs
// from the project root.
//
// sr:docs https://cursor.com/docs/hooks#configuration
func (h *Hooks) Fire(ctx context.Context, e Event, own map[string]any) []Decision {
	var ds []Decision
	payload := h.Common().Payload(e, own)
	for _, entry := range h.Config.Entries(e) {
		run := corehooks.Invoke(ctx, corehooks.Command{Line: entry.Command, Dir: h.Dir, Stdin: payload, Env: h.Env, Strict: entry.FailClosed})
		ds = append(ds, Interpret(e, entry, run))
	}
	return ds
}
