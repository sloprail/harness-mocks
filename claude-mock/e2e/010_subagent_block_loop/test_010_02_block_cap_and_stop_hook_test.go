package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT010_04_BlockCapBoundsTheLoop: a SubagentStop hook that ALWAYS blocks must
// be bounded by CLAUDE_CODE_STOP_HOOK_BLOCK_CAP. With the cap set to 2 the subagent
// runs 3 times (initial + 2 re-runs) then the mock gives up; with the default
// (unset = 8) it runs 9 times. The mock writes a "giving up" line to stderr.
//
// sr:docs https://code.claude.com/docs/en/env-vars
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
// sr:proves stop-block-cap/claude
// staged:proves subagent-stop-block-loop/claude
func TestT010_04_BlockCapBoundsTheLoop(t *testing.T) {
	alwaysBlockExit2 := func(dir, fireCounter string) string {
		return writeHook(t, dir, "stop.sh", `cat >/dev/null
echo "fire" >> "`+fireCounter+`"
echo "still failing" 1>&2
exit 2`)
	}

	t.Run("cap=2 caps at 3 runs", func(t *testing.T) {
		dir := t.TempDir()
		counter := filepath.Join(dir, "subagent-runs.txt")
		fireCounter := filepath.Join(dir, "stop-fires.txt")
		stopHook := alwaysBlockExit2(dir, fireCounter)

		subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
		out, code := driveAgentTool(t, dir, subScript, stopHook,
			[]string{envStopHookBlockCap + "=2"})
		require.Equal(t, 0, code, "mock gives up gracefully (exit 0) when the cap is hit; output:\n%s", out)

		assert.Equal(t, 3, runCount(t, counter),
			"CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=2 must allow initial + 2 re-runs = 3 subagent runs")
		assert.Contains(t, out, "giving up",
			"mock must report giving up on stderr when the block cap is exceeded")
		assert.Contains(t, out, "cap",
			"the give-up message must reference the cap")
	})

	t.Run("default caps at 9 runs", func(t *testing.T) {
		dir := t.TempDir()
		counter := filepath.Join(dir, "subagent-runs.txt")
		fireCounter := filepath.Join(dir, "stop-fires.txt")
		stopHook := alwaysBlockExit2(dir, fireCounter)

		subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
		out, code := driveAgentTool(t, dir, subScript, stopHook, nil) // cap unset → default 8
		require.Equal(t, 0, code, "mock gives up gracefully at the default cap; output:\n%s", out)

		assert.Equal(t, 9, runCount(t, counter),
			"the default CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=8 must allow initial + 8 re-runs = 9 subagent runs")
		assert.Contains(t, out, "giving up", "mock must report giving up at the default cap")
	})
}

// TestT010_05_BlockCapZeroIsUnlimited: CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=0 disables
// the cap. A hook that blocks 9 times (more than the default 8) then stops must
// run the subagent to completion (10 runs) rather than being capped at 9 — proving
// 0 means unlimited, not "block zero times".
//
// sr:docs https://code.claude.com/docs/en/env-vars
// sr:proves stop-block-cap/claude
func TestT010_05_BlockCapZeroIsUnlimited(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "subagent-runs.txt")
	fireCounter := filepath.Join(dir, "stop-fires.txt")

	// Block on the first 9 fires (exit 2), allow on the 10th. 9 > default cap of 8,
	// so under the default this would be capped; with cap=0 it must complete.
	stopHook := writeHook(t, dir, "stop.sh", `cat >/dev/null
echo "fire" >> "`+fireCounter+`"
fires=$(grep -c fire "`+fireCounter+`")
if [ "$fires" -le 9 ]; then
  echo "retry" 1>&2
  exit 2
fi
exit 0`)

	subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
	out, code := driveAgentTool(t, dir, subScript, stopHook,
		[]string{envStopHookBlockCap + "=0"})
	require.Equal(t, 0, code, "mock should exit 0 once the hook finally stops blocking; output:\n%s", out)

	assert.Equal(t, 10, runCount(t, counter),
		"CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=0 must NOT cap: 9 blocks then a clean stop = 10 runs")
	assert.NotContains(t, out, "giving up",
		"with the cap disabled the mock must never report giving up when the hook eventually stops")
}

// TestT010_06_StopHookActiveFlag: stop_hook_active must be false on the FIRST
// SubagentStop fire and true on every subsequent (re-fired) invocation — the real
// Claude flag a hook checks to break its own recursion. The hook captures each
// fire's stop_hook_active value (one line per fire) for the assertion.
//
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
// staged:proves subagent-stop-block-loop/claude
func TestT010_06_StopHookActiveFlag(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "subagent-runs.txt")
	fireCounter := filepath.Join(dir, "stop-fires.txt")
	activeLog := filepath.Join(dir, "stop-active.txt")

	// Capture stop_hook_active per fire. When the field is absent (false) the grep
	// finds nothing → record "false"; when present record its value. Block the
	// first two fires (exit 2), allow the third, so we observe 3 fires:
	// false (first), true (re-fire 1), true (re-fire 2).
	stopHook := writeHook(t, dir, "stop.sh", `input=$(cat)
active=$(echo "$input" | grep -o '"stop_hook_active":[a-z]*' | cut -d':' -f2)
if [ -z "$active" ]; then active=false; fi
echo "$active" >> "`+activeLog+`"
echo "fire" >> "`+fireCounter+`"
fires=$(grep -c fire "`+fireCounter+`")
if [ "$fires" -le 2 ]; then
  echo "retry" 1>&2
  exit 2
fi
exit 0`)

	subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
	out, code := driveAgentTool(t, dir, subScript, stopHook, nil)
	require.Equal(t, 0, code, "mock should exit 0 after the hook stops blocking; output:\n%s", out)

	// 3 runs: initial + 2 re-runs (blocked twice, allowed on the third fire).
	assert.Equal(t, 3, runCount(t, counter), "two blocks then a clean stop must yield 3 subagent runs")

	active := nonEmptyLines(readFile(t, activeLog))
	require.Len(t, active, 3, "SubagentStop must fire 3 times; got %q", active)
	assert.Equal(t, "false", active[0], "stop_hook_active must be FALSE on the first SubagentStop fire")
	assert.Equal(t, "true", active[1], "stop_hook_active must be TRUE on the first re-fire (after a block)")
	assert.Equal(t, "true", active[2], "stop_hook_active must be TRUE on every subsequent re-fire")
}

// TestT010_07_BlockedSubagentStopSurfacesAsAttachment: a SubagentStop hook's
// refusal must reach the SUB-AGENT's transcript as a hook_blocking_error
// attachment carrying its TEXT, preceded by the "Stop hook feedback" turn — the
// same channel the root's Stop uses in the session's own file. Real Claude Code
// writes a SubagentStop's feedback and attachment into the sub-agent's
// subagents/agent-<id>.jsonl: every SubagentStop attachment in the real
// transcripts on one machine (40 files) is in a sidechain file, none in a main
// one.
//
// The re-run loop alone is not an observable consequence for anything outside
// the mock. It changes how many times the subagent script runs, which only the
// subagent's own side effects reveal; the orchestrator sees the same
// tool_result either way, and a consumer reading the conversation cannot tell a
// refused sub-agent cycle from a clean one. Without this, the only trace a
// SubagentStop refusal left was a line on the mock's stderr — a diagnostic, not
// a record — so a guardrail refusing at the end of a delegated cycle had no
// channel by which its words reached the conversation it was judging.
//
// The two blocking forms are recorded differently, as claude 2.1.282 records
// them (a controlled run, EVIDENCE.md): an exit-0 decision:block leaves the
// "Stop hook feedback:\n<reason>" turn AND a hook_blocking_error attachment
// {blockingError: {blockingError: reason, command}}; an exit 2 leaves only the
// feedback turn, quoting the hook as "[<command>]: <stderr>".
//
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
// sr:proves hook-output-transcript-records/claude
// staged:proves subagent-stop-block-loop/claude
func TestT010_07_BlockedSubagentStopSurfacesAsAttachment(t *testing.T) {
	for _, tc := range []struct {
		name       string
		hookBody   string
		want       string
		quoted     bool // exit 2: the feedback quotes "[<command>]: <stderr>"
		attachment bool
	}{
		{
			name: "exit 2 with text on stderr",
			want: "verify failed: the delegated work is refused",
			hookBody: `cat >/dev/null
echo "verify failed: the delegated work is refused" 1>&2
exit 2`,
			quoted: true,
		},
		{
			name: "exit 0 with decision block",
			want: "a10n://check-runs/xyz needs resolving before this subagent may stop",
			hookBody: `cat >/dev/null
printf '%s' '{"decision":"block","reason":"a10n://check-runs/xyz needs resolving before this subagent may stop"}'
exit 0`,
			attachment: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			configDir := filepath.Join(dir, "config")
			counter := filepath.Join(dir, "subagent-runs.txt")

			stopHook := writeHook(t, dir, "stop.sh", tc.hookBody)
			subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
			writeSettings(t, dir, map[string]string{"SubagentStop": stopHook})
			orch := writeScript(t, dir, "orch.sh", orchestratorScript(subScript))

			// Cap the loop at 1 so the always-blocking hook terminates quickly;
			// the assertion is about the record, not about how far it looped.
			out, code := runInDir(t, dir, []string{envStopHookBlockCap + "=1"},
				"--script", orch, "--session-id", "sess-stop-attach",
				"--project-dir", dir, "--config-dir", configDir, "-p", "go")
			require.Equal(t, 0, code, "mock gives up gracefully at the cap; output:\n%s", out)
			require.GreaterOrEqual(t, runCount(t, counter), 2,
				"the hook blocked, so the subagent turn must have been re-run")

			main := readTranscript(t, configDir, dir, "sess-stop-attach")
			assert.NotContains(t, main, `"hookEvent":"SubagentStop"`,
				"a SubagentStop's attachment belongs to the sub-agent's own file, not the dispatcher's")
			transcript := readSidechains(t, configDir, dir, "sess-stop-attach")
			feedback := `Stop hook feedback:\n` + tc.want
			if tc.quoted {
				feedback = `Stop hook feedback:\n[` + stopHook + `]: ` + tc.want + `\n`
			}
			assert.Contains(t, transcript, feedback,
				"the re-run sub-agent reads the refusal as a Stop hook feedback turn")
			if tc.attachment {
				assert.Contains(t, transcript, `"type":"hook_blocking_error"`)
				assert.Contains(t, transcript, `"hookEvent":"SubagentStop"`,
					"the attachment must name SubagentStop, so a reader can tell which cycle refused")
				assert.Contains(t, transcript, `"blockingError":{"blockingError":"`+tc.want+`","command":"`+stopHook+`"}`)
			} else {
				assert.NotContains(t, transcript, "hook_blocking_error", "an exit-2 block leaves no attachment")
			}
		})
	}
}

// TestT010_08_CleanSubagentStopWritesNoAttachment: the companion constraint. A
// SubagentStop that does not block must leave no blocking-error attachment, or
// the attachment stops meaning "refused" and every delegated cycle looks
// refused — which would make the assertion above pass for a broken mock.
func TestT010_08_CleanSubagentStopWritesNoAttachment(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	counter := filepath.Join(dir, "subagent-runs.txt")

	stopHook := writeHook(t, dir, "stop.sh", `cat >/dev/null
exit 0`)
	subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
	writeSettings(t, dir, map[string]string{"SubagentStop": stopHook})
	orch := writeScript(t, dir, "orch.sh", orchestratorScript(subScript))

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "sess-stop-clean",
		"--project-dir", dir, "--config-dir", configDir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	require.Equal(t, 1, runCount(t, counter), "a clean SubagentStop must not re-run the subagent")

	assert.NotContains(t, readTranscript(t, configDir, dir, "sess-stop-clean"), "hook_blocking_error",
		"a SubagentStop that allowed the stop must record no blocking error")
}
