package e2e

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-matchers-after-events: matchers on
// afterShellExecution (a command pattern, a pattern that matches nothing and
// the empty string) and on postToolUseFailure (Shell, Read, Write and the empty
// string), and an empty-string matcher on preToolUse, over two commands that
// succeed, one that fails and a Read of a file that is not there.

// rans are the hooks that ran for an event's subject (its command, or its tool
// when it has none), by the name the scenario's hook script was given.
func rans(results []string, event, subject string) []string {
	var out []string
	for _, r := range results {
		p := strings.Split(r, ":")
		if p[0] != "ran" || len(p) < 6 || p[2] != event {
			continue
		}
		if p[4] == subject || (p[4] == "" && p[3] == subject) {
			out = append(out, p[1])
		}
	}
	sort.Strings(out)
	return out
}

// TestAfterShellExecutionHooksAreMatchedOnTheCommandLine: recorded, an
// afterShellExecution hook is matched against the full command string, as the
// docs say: "echo MATCH" ran after echo MATCH-ME only, a matcher that matches no
// command never ran, and the empty-string matcher ran after every command,
// failing ones included.
// sr:proves hook-matcher-filter/cursor
func TestAfterShellExecutionHooksAreMatchedOnTheCommandLine(t *testing.T) {
	got, want := replay(t, "hook-matchers-after-events")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		require.Equal(t, []string{"after-MATCH", "after-empty"}, rans(o.results, "afterShellExecution", "echo MATCH-ME"), name)
		require.Equal(t, []string{"after-empty"}, rans(o.results, "afterShellExecution", "echo OTHER"), name)
		require.Equal(t, []string{"after-empty"}, rans(o.results, "afterShellExecution", "false"), name+": a failed command raises the event too")
		for _, r := range o.results {
			require.NotContains(t, r, "after-OTHER", name+": its matcher matches no command")
		}
	}
}

// TestPostToolUseFailureHooksAreMatchedOnTheToolName: recorded, a
// postToolUseFailure hook is matched against the tool's name: a failed command
// ran the Shell hook and not the Read or Write one, a failed read the Read hook
// only, and the empty-string matcher ran for both.
// sr:proves hook-matcher-filter/cursor
func TestPostToolUseFailureHooksAreMatchedOnTheToolName(t *testing.T) {
	got, want := replay(t, "hook-matchers-after-events")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		require.Equal(t, []string{"fail-Shell", "fail-empty"}, rans(o.results, "postToolUseFailure", "false"), name+": the failed command")
		require.Equal(t, []string{"fail-Read", "fail-empty"}, rans(o.results, "postToolUseFailure", "Read"), name)
		for _, r := range o.results {
			require.NotContains(t, r, "fail-Write", name+": no Write call failed")
		}
	}
}

// TestAnEmptyStringMatcherMatchesEveryCall: recorded, a hook whose matcher is
// the empty string runs for every tool call, as one with no matcher does ("An
// empty string or "*" matches everything").
// sr:proves hook-matcher-filter/cursor
func TestAnEmptyStringMatcherMatchesEveryCall(t *testing.T) {
	got, want := replay(t, "hook-matchers-after-events")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for _, subject := range []string{"echo MATCH-ME", "echo OTHER", "false"} {
			require.Equal(t, []string{"pre-empty"}, rans(o.results, "preToolUse", subject), name+": "+subject)
		}
		require.Equal(t, []string{"pre-empty"}, rans(o.results, "preToolUse", "Read"), name)
	}
}
