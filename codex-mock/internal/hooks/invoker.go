package hooks

import (
	"context"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Invoker fires the hooks of one session.
type Invoker struct {
	Config Config
	// Dir is the session's working directory, where commands run.
	Dir string
	// Environ is the environment the mock was started with; Ident and
	// Defaults are the session facts Codex adds to a hook command's (none).
	Environ         []string
	Ident, Defaults map[string]string
	Common          Common
}

// DefaultTimeout is Codex's hook timeout when none is configured, for every
// event but SessionEnd; SessionEnd's is SessionEndDefault, and no hook of it
// may run longer than SessionEndCap, whatever its own timeout says.
//
// sr:docs https://developers.openai.com/codex/hooks#config-shape
const (
	DefaultTimeout    = 600 * time.Second
	SessionEndDefault = 1 * time.Second
	SessionEndCap     = 3 * time.Second
)

// timeoutFor is the timeout a hook of ev runs under: its own (zero when none
// is configured, which the core replaces by the default), capped for SessionEnd.
func timeoutFor(ev Event, own time.Duration) time.Duration {
	if ev == SessionEnd {
		return corehooks.CapTimeout(corehooks.DefaultTimeout(own, SessionEndDefault), SessionEndCap)
	}
	return own
}

// toolAliases are the other names a canonical tool name also matches.
//
// sr:docs https://developers.openai.com/codex/hooks#matcher-patterns
var toolAliases = map[string][]string{"apply_patch": {"Edit", "Write"}}

// Fire runs every command of every group of the event whose matcher selects
// match, all at once (Codex launches matching hooks concurrently), and returns
// their outcomes in configuration order. The payload carries the common
// fields and the event's own. UserPromptSubmit and Stop ignore a matcher.
//
// sr:docs https://developers.openai.com/codex/hooks#review-and-trust-hooks
// sr:provides hook-command-handler/codex
// sr:provides hook-matcher-filter/codex
// sr:provides hook-timeout/codex
func (iv *Invoker) Fire(ctx context.Context, ev Event, match string, own map[string]any) []corehooks.Outcome {
	var cmds []corehooks.Command
	for _, g := range iv.Config[ev] {
		if ev == UserPromptSubmit || ev == Stop || corehooks.Matches(g.Matcher, match, toolAliases[match]...) {
			for _, h := range g.Handlers {
				cmds = append(cmds, corehooks.Command{Line: h.Command, Timeout: timeoutFor(ev, time.Duration(h.Timeout)*time.Second)})
			}
		}
	}
	return corehooks.RunAll(ctx, cmds, Payload(iv.Common, ev, own), corehooks.Runtime{
		Dir: iv.Dir, Env: procexec.Env(iv.Environ, iv.Ident, iv.Defaults), DefaultTimeout: DefaultTimeout})
}
