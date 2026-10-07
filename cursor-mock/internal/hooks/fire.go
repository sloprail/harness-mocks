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
	// Env is the environment a hook command starts with, at the time of the event.
	Env func() []string
	// Common builds the fields every payload carries, at the time of the event.
	Common func() Common
}

// NoSubject is the subject of an event that has none (sessionStart and
// sessionEnd): every hook of the event runs, whatever its matcher.
const NoSubject = "\x00"

// Fire runs the commands configured for the event whose matcher selects its
// subject (the tool's name for a tool event, the command line for a shell
// event, "Write" for a file edit; an event with NoSubject takes every hook),
// all at once (Cursor runs an event's hooks side by side), each bounded by its
// timeout, with the event's payload on stdin. It returns what each decided, in
// the order configured. A project hook runs from the project root.
//
// sr:provides hook-command-handler/cursor
// sr:provides hook-matcher-filter/cursor
// sr:provides hooks-all-matching-run/cursor
// sr:docs https://cursor.com/docs/hooks#configuration
func (h *Hooks) Fire(ctx context.Context, e Event, subject string, own map[string]any) []Decision {
	var entries []Entry
	var cmds []corehooks.Command
	for _, entry := range h.Config.Entries(e) {
		if subject == NoSubject || corehooks.Matches(entry.Matcher, subject) {
			entries = append(entries, entry)
			c := corehooks.Command{Line: entry.Command, Timeout: entry.Timeout}
			if entry.PluginRoot != "" { // a plugin's hook runs in the plugin, which it is told of (runs/plugin-hook-cwd-env)
				c.Dir, c.Env = entry.PluginRoot, []string{"CURSOR_PLUGIN_ROOT=" + entry.PluginRoot}
			}
			cmds = append(cmds, c)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	outs := corehooks.RunAll(ctx, cmds, h.Common().Payload(e, own), corehooks.Runtime{Dir: h.Dir, Env: h.Env()})
	ds := make([]Decision, len(outs))
	for i, o := range outs {
		ds[i] = Interpret(e, entries[i], o)
	}
	return ds
}
