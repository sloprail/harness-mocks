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

// Fire runs every command configured for the event, all at once (Cursor runs
// an event's hooks side by side), with the event's payload on stdin, and
// returns what each decided, in the order configured. A project hook runs from
// the project root.
//
// sr:docs https://cursor.com/docs/hooks#configuration
func (h *Hooks) Fire(ctx context.Context, e Event, own map[string]any) []Decision {
	entries := h.Config.Entries(e)
	if len(entries) == 0 {
		return nil
	}
	cmds := make([]corehooks.Command, len(entries))
	for i, entry := range entries {
		cmds[i] = corehooks.Command{Line: entry.Command}
	}
	outs := corehooks.RunAll(ctx, cmds, h.Common().Payload(e, own), corehooks.Runtime{Dir: h.Dir, Env: h.Env})
	ds := make([]Decision, len(outs))
	for i, o := range outs {
		ds[i] = Interpret(e, entries[i], o)
	}
	return ds
}
