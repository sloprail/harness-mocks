package hooks

import (
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// StartContext is the developer context a SubagentStart hook gave the
// sub-agent: what it printed as additionalContext, or else, when it exited 0
// without an error and printed anything but a JSON object, its plain output
// (hooks#subagentstart).
func StartContext(o corehooks.Outcome) string {
	d := Interpret(SubagentStart, o)
	if d.Context != "" {
		return d.Context
	}
	if o.Exit == 0 && d.Error == "" && !strings.HasPrefix(strings.TrimSpace(o.Stdout), "{") {
		return strings.TrimSpace(o.Stdout)
	}
	return ""
}
