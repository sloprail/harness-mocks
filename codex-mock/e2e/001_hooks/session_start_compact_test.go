package e2e

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A session-start hook that says continue:false after a compaction does not
// stop the session, but ends the turn, cleanly: the compaction was made, the
// hook ran, and then no further command ran, no Stop hook fired and the model
// was not asked again; the stream completes the turn and the run exits 0 (not
// the abort a stopped PreCompact or PostCompact is). Recorded with the real
// harness under runs/session-start-compact-continue-false, whose hook printed
// continue:false only for source "compact", and replayed here.
// sr:proves session-start-hook/codex
func TestSessionStartAfterCompactionContinueFalseEndsTheTurn(t *testing.T) {
	rec, got := replayCompacting(t, "session-start-compact-continue-false")
	require.Equal(t, 0, got.Code, got.Stderr)

	want := hookShape(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Equal(t, []string{"SessionStart:startup", "PreCompact:auto", "PostCompact:auto", "SessionStart:compact", "SessionEnd"}, want, "recorded")
	assert.Equal(t, want, hookShape(got.hookLog()), "the compact start was the last event before the session ended: no Stop hook")
	assert.Equal(t, "0\n", readFile(t, filepath.Join(rec.sample, "exit.txt")))

	assert.Equal(t, 1, compactedRecords(got.rollout(t)), "the compaction was made")
	assert.NotContains(t, got.rollout(t), `"reason":"interrupted"`, "not an aborted turn")
	assert.Equal(t, 1, strings.Count(got.Stdout, `"turn.completed"`))
	assert.Equal(t, 1, strings.Count(readFile(t, filepath.Join(rec.sample, "stream.jsonl")), `"turn.completed"`))
	cmds, _ := got.commands()
	assert.Len(t, cmds, 1, "no command after the one that led to the compaction (the recording ran one of three)")
}

// After a compaction the session starts again with source "compact", and a
// session-start hook's matcher is applied to that source (hooks#sessionstart):
// the recorded run (runs/manual-compaction-auto) fired SessionStart:compact
// after each of its three compactions and the mock does the same, with the
// payload naming the source and the session; a matcher of "compact" (or any
// that matches it) runs for it, one matching only startup, resume or clear
// does not.
// sr:proves session-start-hook/codex
func TestSessionStartAfterCompactionHasSourceCompactAndMatches(t *testing.T) {
	rec, got := replayCompacting(t, "manual-compaction-auto")
	require.Equal(t, 0, got.Code, got.Stderr)
	for who, log := range map[string][]map[string]any{
		"recording": jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))), "mock": got.hookLog()} {
		n := 0
		for _, l := range log {
			if l["hook_event_name"] == "SessionStart" && l["source"] == "compact" {
				n++
				assert.NotEmpty(t, l["session_id"], who)
			}
		}
		assert.Equal(t, 3, n, "%s: one start after each compaction", who)
		assert.Equal(t, "startup", log[0]["source"], who)
	}

	for matcher, runs := range map[string]bool{
		"compact": true, "": true, "*": true, "^compact$": true, "startup|resume|clear|compact": true,
		"startup": false, "resume": false, "clear": false, "startup|resume|clear": false,
	} {
		t.Run("matcher="+matcher, func(t *testing.T) {
			hooks := `{"hooks":{"SessionStart":[{"matcher":` + strconv.Quote(matcher) +
				`,"hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh"}]}]}}`
			r := execMock(t, scenario{
				HooksJSON: hooks,
				Files:     map[string]string{"hook.sh": "#!/bin/sh\ncat >>\"$HOOK_LOG\"\necho >>\"$HOOK_LOG\"\n"},
				Script: `#!/bin/sh
c=$(grep -c '"type":"compacted"' "$A10N_MOCK_SESSION_FILE")
case "$c" in
0) printf '%s\n' '{"type":"compact","trigger":"auto"}' ;;
*) printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}' ;;
esac
`,
				Prompt: "compact",
			})
			require.Equal(t, 0, r.Code, r.Stderr)
			var sources []string
			for _, l := range r.hookLog() {
				sources = append(sources, l["source"].(string))
			}
			if runs && matcher != "startup|resume|clear|compact" && matcher != "" && matcher != "*" {
				assert.Equal(t, []string{"compact"}, sources)
			} else if runs {
				assert.Contains(t, sources, "compact")
			} else {
				assert.NotContains(t, sources, "compact")
			}
		})
	}
}

// A PostCompact hook's matcher selects on the trigger as PreCompact's does
// (hooks#postcompact): one matching "manual" fires for the manual compaction,
// carrying that trigger, and not for the auto one.
// sr:proves manual-compaction/codex
func TestPostCompactMatcherSelectsOnTheTrigger(t *testing.T) {
	hooks := `{"hooks":{"PostCompact":[{"matcher":"^manual$","hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh"}]}]}}`
	got := execMock(t, scenario{
		HooksJSON: hooks,
		Files:     map[string]string{"hook.sh": "#!/bin/sh\ncat >>\"$HOOK_LOG\"\necho >>\"$HOOK_LOG\"\n"},
		Script: `#!/bin/sh
c=$(grep -c '"type":"compacted"' "$A10N_MOCK_SESSION_FILE")
case "$c" in
0) printf '%s\n' '{"type":"compact"}' ;;
1) printf '%s\n' '{"type":"compact","trigger":"auto"}' ;;
*) printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}' ;;
esac
`,
		Prompt: "compact",
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, []string{"PostCompact:manual"}, hookShape(got.hookLog()), "only the manual trigger matched")
	assert.Equal(t, 2, compactedRecords(got.rollout(t)))
}
