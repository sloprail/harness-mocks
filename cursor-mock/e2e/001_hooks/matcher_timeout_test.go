package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-matchers: hooks with matchers on preToolUse,
// beforeShellExecution, postToolUse, afterFileEdit and sessionStart.

// TestAHookRunsOnlyForAnEventWhoseSubjectMatchesItsMatcher: recorded, on a
// Shell preToolUse the hooks matching "Shell", "Sh.*|Rea", "*" and no matcher
// ran, and those for "Read" and "Write" did not; on a Read, "Read" ran and
// "Shell" did not; a shell event is matched on its command line.
// sr:proves hook-matcher-filter/cursor
func TestAHookRunsOnlyForAnEventWhoseSubjectMatchesItsMatcher(t *testing.T) {
	got, want := replay(t, "hook-matchers")
	conforms(t, got, want)

	var shell, read, onShell, onEcho []string
	for _, r := range got.results {
		p := strings.Split(r, ":")
		if p[0] != "ran" || len(p) < 6 {
			continue
		}
		switch {
		case p[2] == "preToolUse" && p[3] == "Shell" && p[4] == "echo MATCH-ME":
			shell = append(shell, p[1])
		case p[2] == "preToolUse" && p[3] == "Read":
			read = append(read, p[1])
		case p[2] == "beforeShellExecution":
			onEcho = append(onEcho, p[1]+"@"+p[4])
		case p[2] == "afterFileEdit":
			onShell = append(onShell, p[1])
		}
	}
	require.ElementsMatch(t, []string{"pre-Shell", "pre-none", "pre-regex", "pre-star"}, shell)
	require.Contains(t, read, "pre-Read")
	require.NotContains(t, read, "pre-Shell")
	require.Equal(t, []string{"shell-MATCH@echo MATCH-ME"}, onEcho, "echo OTHER matches neither beforeShellExecution matcher")
	require.Equal(t, []string{"edit-Write"}, onShell)
}

// TestAnEventWithNoSubjectTakesEveryHookWhateverItsMatcher: recorded, a
// sessionStart hook whose matcher matches nothing still runs.
// sr:proves hook-matcher-filter/cursor
func TestAnEventWithNoSubjectTakesEveryHookWhateverItsMatcher(t *testing.T) {
	got, want := replay(t, "hook-matchers")
	conforms(t, got, want)
	require.Contains(t, got.results, "ran:nomatch-session-start:sessionStart:::<nil>")
}

// The recorded run runs/hook-timeout: a hook with a 1 s timeout that runs for
// 5 s and has a child that would log after 3 s.

// TestAHookPastItsTimeoutIsKilledWithItsChildrenAndTheCommandGoesOn:
// recorded, the hook and the child it spawned are killed (the child never
// logs), what the hook would have printed is discarded, and the command runs.
// sr:proves hook-timeout/cursor
func TestAHookPastItsTimeoutIsKilledWithItsChildrenAndTheCommandGoesOn(t *testing.T) {
	got, want := replay(t, "hook-timeout")
	conforms(t, got, want)

	require.Contains(t, got.results, "ran:SLOWOPEN:<nil>:<nil>:<nil>:started")
	for _, r := range got.results {
		require.NotContains(t, r, "grandchild-finished", "the child was killed with the hook")
		require.NotContains(t, r, ":finished", "the hook was killed before it finished")
	}
	var after []string
	for _, h := range got.hooks {
		if h["hook_event_name"] == "afterShellExecution" {
			after = append(after, h["command"].(string))
		}
	}
	require.Contains(t, after, "echo SLOWOPEN", "the command ran")
	require.NotContains(t, after, "echo SLOWCLOSED", "the fail-closed one did not")
}

// TestATimedOutHookThatFailsClosedBlocksTheCommand: recorded, with failClosed a
// hook that times out blocks the command, and says it timed out.
// sr:proves hook-timeout/cursor
func TestATimedOutHookThatFailsClosedBlocksTheCommand(t *testing.T) {
	got, want := replay(t, "hook-timeout")
	conforms(t, got, want)

	msg, kind, ok := failureOf(got, "echo SLOWCLOSED")
	require.True(t, ok)
	require.Equal(t, "permission_denied", kind)
	require.Contains(t, msg, "Hook script timed out after 1000ms")
	require.Contains(t, msg, "configured to fail closed")
}

// TestABeforeReadFileHookIsMatchedOnTheToolName: the docs say a beforeReadFile
// hook is matched against the tool, Read (https://cursor.com/docs/hooks#matcher-configuration);
// no recording configures a matcher on it, so this drives the mock: the hook
// whose matcher is Read runs for a Read, the one whose matcher is Shell does not.
func TestABeforeReadFileHookIsMatchedOnTheToolName(t *testing.T) {
	log := `#!/bin/sh
cat >/dev/null
echo "$0" >>"$HOOK_LOG"
`
	r := runTools(t, `{"version":1,"hooks":{"beforeReadFile":[{"command":".cursor/hooks/on-read.sh","matcher":"Read"},{"command":".cursor/hooks/on-shell.sh","matcher":"Shell"}]}}`,
		map[string]string{"on-read.sh": log, "on-shell.sh": log},
		map[string]string{"note.txt": "hi\n"},
		map[string]any{"name": "Read", "input": map[string]any{"file_path": "note.txt"}})
	got := r.logged(t)
	require.Contains(t, got, "on-read.sh")
	require.NotContains(t, got, "on-shell.sh")
}
