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

// ranFor are the hooks that ran for an event on a tool's calls, each once,
// whatever the number of calls, by the name the scenario's hook script was given.
func ranFor(results []string, event, tool string) []string {
	seen := map[string]bool{}
	for _, r := range results {
		p := strings.Split(r, ":")
		if p[0] == "ran" && len(p) >= 6 && p[2] == event && p[3] == tool {
			seen[p[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// The recorded run runs/hook-matchers has, on preToolUse, matchers for one
// tool (Shell, Read, Write), a pattern ("Sh.*|Rea"), "*" and none; on
// postToolUse matchers for Shell and Read; two Shell calls, a Write and two
// Reads.

// TestEachToolEventRunsTheHooksWhoseMatcherSelectsItsTool: recorded, a tool
// event runs exactly the hooks whose matcher selects the tool's name: a
// preToolUse for Shell runs the Shell, pattern, "*" and unmatched hooks; for
// Write the Write, "*" and unmatched hooks, but not the pattern "Sh.*|Rea",
// which matches neither name; for Read the Read, "*" and unmatched hooks and
// the pattern, which selects Read by its unanchored part "Rea". postToolUse
// runs the Shell hook after the Shell calls, the Read hook after the Reads and
// none after the Write.
// sr:proves hook-matcher-filter/cursor
func TestEachToolEventRunsTheHooksWhoseMatcherSelectsItsTool(t *testing.T) {
	got, want := replay(t, "hook-matchers")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		require.Equal(t, []string{"pre-Shell", "pre-none", "pre-regex", "pre-star"}, ranFor(o.results, "preToolUse", "Shell"), name)
		require.Equal(t, []string{"pre-Write", "pre-none", "pre-star"}, ranFor(o.results, "preToolUse", "Write"), name+": the pattern selects neither Write nor Sh.*")
		require.Equal(t, []string{"pre-Read", "pre-none", "pre-regex", "pre-star"}, ranFor(o.results, "preToolUse", "Read"), name+": the pattern selects Read by a partial match")
		require.Equal(t, []string{"post-Shell"}, ranFor(o.results, "postToolUse", "Shell"), name)
		require.Equal(t, []string{"post-Read"}, ranFor(o.results, "postToolUse", "Read"), name)
		require.Empty(t, ranFor(o.results, "postToolUse", "Write"), name+": no postToolUse hook is configured for Write")
		// the hooks without a matcher or with "*" ran on every one of the five calls
		var star, none int
		for _, r := range o.results {
			if strings.HasPrefix(r, "ran:pre-star:preToolUse") {
				star++
			}
			if strings.HasPrefix(r, "ran:pre-none:preToolUse") {
				none++
			}
		}
		require.Equal(t, 5, star, name)
		require.Equal(t, 5, none, name)
	}
}
