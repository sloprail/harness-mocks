package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs runs/manual-compaction-auto and runs/manual-compaction-auto-blocked:
// a turn of three commands under a context limit small enough that Codex
// compacts after each of the first three tool results, with a hook on every
// compaction event; and the same turn with a PreCompact hook printing
// `continue: false`. A `codex exec` has no /compact (runs/manual-compaction-slash:
// the text goes to the model), so the scenario asks for a compaction with a
// compact record, the one place the mock departs from the recording.
const compactingScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
c=$(grep -c '"type":"compacted"' "$A10N_MOCK_SESSION_FILE")
asked=$(cat "$TMPDIR/asked" 2>/dev/null || echo 0)
if [ "$asked" -lt "$n" ]; then
  echo "$n" >"$TMPDIR/asked"
  printf '%s\n' '{"type":"compact","trigger":"auto"}'
  exit 0
fi
cmd=$(sed -n "$((n+1))p" "$CALLS")
if [ -n "$cmd" ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_%s","name":"Bash","input":{"command":%s}}]}}\n' "$n" "$(printf '%s' "$cmd" | jq -Rs .)"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`

func replayCompacting(t *testing.T, name string) (recording, result) {
	t.Helper()
	rec := loadRecording(t, name)
	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    compactingScript,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       withCalls(t, rec.calls...),
	})
	return rec, got
}

// hookShape is the hooks that fired, in order, as the name and what tells the
// compactions apart (the trigger, the start's source).
func hookShape(lines []map[string]any) (out []string) {
	for _, l := range lines {
		s, _ := l["hook_event_name"].(string)
		for _, k := range []string{"trigger", "source"} {
			if v, ok := l[k].(string); ok {
				s += ":" + v
			}
		}
		out = append(out, s)
	}
	return
}

func compactedRecords(rollout string) int { return strings.Count(rollout, `"type":"compacted"`) }

// Compacting fires PreCompact with the trigger, then records the compaction,
// then fires PostCompact, then SessionStart with source "compact"; each of
// them as the recording shows it: the payload names the turn and the trigger,
// and no permission mode.
// sr:proves manual-compaction/codex
func TestCompactionFiresTheHooksAroundIt(t *testing.T) {
	rec, got := replayCompacting(t, "manual-compaction-auto")
	require.Equal(t, 0, got.Code, got.Stderr)

	want := jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	assert.Equal(t, hookShape(want), hookShape(got.hookLog()))
	assert.Equal(t, 3, compactedRecords(got.rollout(t)), "one compaction record each")
	for _, l := range got.hookLog() {
		switch l["hook_event_name"] {
		case "PreCompact", "PostCompact":
			assert.Contains(t, l, "turn_id")
			assert.Contains(t, l, "transcript_path")
			assert.NotContains(t, l, "permission_mode", "a compaction's payload has none")
		}
	}
	// the turn goes on past each compaction and ends once
	cmds, _ := got.commands()
	assert.Len(t, cmds, 3)
	assert.Equal(t, 1, strings.Count(got.Stdout, `"turn.completed"`))
}

// A PreCompact hook that prints `continue: false` stops the compaction: it is
// not recorded, and neither PostCompact nor the session start fires. Codex
// then aborts the whole turn: the rollout records it interrupted, no Stop hook
// fires, the stream has no turn.completed and the run fails (recorded:
// runs/manual-compaction-auto-blocked).
// sr:proves manual-compaction/codex
func TestPreCompactCanStopTheCompaction(t *testing.T) {
	rec, got := replayCompacting(t, "manual-compaction-auto-blocked")

	want := hookShape(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Equal(t, []string{"SessionStart:startup", "PreCompact:auto"}, want, "recorded")
	assert.Equal(t, want, hookShape(got.hookLog()), "the hooks that fired: no PostCompact, no session start, no Stop")
	assert.Zero(t, compactedRecords(got.rollout(t)), "nothing was compacted")
	assert.Contains(t, got.rollout(t), `"reason":"interrupted"`)
	assert.NotContains(t, got.Stdout, "turn.completed")
	assert.Equal(t, readFile(t, filepath.Join(rec.sample, "exit.txt")), "1\n")
	assert.Equal(t, 1, got.Code)
}

// A PostCompact hook that prints `continue: false` stops what follows the
// compaction: it was recorded, but no session start fires, and Codex aborts the
// turn the same way; the session still ends (runs/manual-compaction-auto-post-stopped).
// sr:proves manual-compaction/codex
func TestPostCompactCanStopWhatFollowsTheCompaction(t *testing.T) {
	rec, got := replayCompacting(t, "manual-compaction-auto-post-stopped")

	want := hookShape(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Equal(t, []string{"SessionStart:startup", "PreCompact:auto", "PostCompact:auto", "SessionEnd"}, want, "recorded")
	assert.Equal(t, want, hookShape(got.hookLog()))
	assert.Equal(t, 1, compactedRecords(got.rollout(t)), "the compaction was recorded")
	assert.Contains(t, got.rollout(t), `"reason":"interrupted"`)
	assert.NotContains(t, got.Stdout, "turn.completed")
	assert.Equal(t, 1, got.Code)
}

// A compaction nobody stops is compacted manually when the scenario asks for
// no trigger, and the hooks' matcher selects on the trigger.
// sr:proves manual-compaction/codex
func TestCompactionHookMatcherSelectsOnTheTrigger(t *testing.T) {
	hooks := `{"hooks":{"PreCompact":[{"matcher":"^manual$","hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh"}]}]}}`
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
	assert.Equal(t, []string{"PreCompact:manual"}, hookShape(got.hookLog()), "only the manual trigger matched")
	assert.Equal(t, 2, compactedRecords(got.rollout(t)))
}
