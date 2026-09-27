// Package e2e holds the SubagentStop block-loop regression tests for the
// a10n-claude-mock binary. They pin the contract that a blocking SubagentStop
// hook RE-RUNS the same subagent turn (it does not hand control back to the
// orchestrator), that a block is signalled BOTH by exit 2 AND by an exit-0
// {"decision":"block"} stdout frame, that the loop is capped by the real Claude
// Code CLAUDE_CODE_STOP_HOOK_BLOCK_CAP env var (default 8, 0 = unlimited), and
// that stop_hook_active is false on the first fire and true on every re-fire.
//
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
// sr:docs https://code.claude.com/docs/en/env-vars
package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envStopHookBlockCap mirrors the REAL Claude Code env var that caps how many
// consecutive times a Stop/SubagentStop hook may block before Claude overrides.
// sr:docs https://code.claude.com/docs/en/env-vars
const envStopHookBlockCap = "CLAUDE_CODE_STOP_HOOK_BLOCK_CAP"

// writeSettings writes a .claude/settings.json wiring one command hook per event.
func writeSettings(t *testing.T, dir string, events map[string]string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	body := `{"hooks":{`
	first := true
	for evt, hookPath := range events {
		if !first {
			body += ","
		}
		first = false
		body += `"` + evt + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hookPath + `"}]}]`
	}
	body += `}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(body), 0o644))
}

// writeHook writes an executable hook script (a POSIX-sh body) and returns its path.
func writeHook(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return p
}

// writeScript writes an executable scenario script and returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o755))
	return p
}

// orchestratorScript emits an Agent tool_use that spawns the subagent scenario at
// subScript, then emits a final result once the synthesised tool_result (carrying
// the literal "agentId:") lands in the session file. The session file is the
// mock's turn-to-turn memory ($A10N_MOCK_SESSION_FILE). Mirrors 009_agent_tool.
func orchestratorScript(subScript string) string {
	return fmt.Sprintf(`#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
printf '%%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_agent_1","name":"Agent","input":{"description":"Implement task 2","prompt":"run a10n-task-executor workspace ensure --task-id 2","subagent_type":"general-purpose","isolation":"worktree","script":"%s"}}]}}'
`, subScript)
}

// countingSubagent returns a subagent scenario that appends one line to counter
// every time it runs, then emits a clean result frame. Counting the lines yields
// the number of times the subagent turn was (re-)run.
func countingSubagent(counter string) string {
	return `#!/bin/sh
echo "run" >> "` + counter + `"
printf '%s\n' '{"type":"result","subtype":"success","result":"subagent done","is_error":false}'
`
}

// runCount returns the number of non-empty lines the subagent appended to counter.
func runCount(t *testing.T, counter string) int {
	t.Helper()
	data, err := os.ReadFile(counter)
	require.NoError(t, err, "subagent must have run at least once (counter file missing)")
	return len(nonEmptyLines(string(data)))
}

// driveAgentTool runs the mock with the standard Agent-tool orchestrator + the
// given subagent script and SubagentStop hook, returning combined output + code.
// Extra env (e.g. CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=2) is forwarded to the process.
func driveAgentTool(t *testing.T, dir, subScript, stopHook string, env []string) (string, int) {
	t.Helper()
	writeSettings(t, dir, map[string]string{"SubagentStop": stopHook})
	orch := writeScript(t, dir, "orch.sh", orchestratorScript(subScript))
	return runInDir(t, dir, env,
		"--script", orch, "--session-id", "sess-block-loop", "--project-dir", dir, "-p", "go")
}

// TestT010_01_BlockViaExit2RerunsSubagent: a SubagentStop hook that blocks via
// exit 2 on its FIRST fire and allows (exit 0) on its SECOND must cause the mock
// to re-run the SAME subagent turn. The subagent runs twice and the hook fires
// twice with the SAME agent_id.
//
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
func TestT010_01_BlockViaExit2RerunsSubagent(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "subagent-runs.txt")
	fireCounter := filepath.Join(dir, "stop-fires.txt")
	idLog := filepath.Join(dir, "stop-ids.txt")

	// Block (exit 2) on the first fire, allow (exit 0) on every later fire.
	stopHook := writeHook(t, dir, "stop.sh", `input=$(cat)
echo "$input" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4 >> "`+idLog+`"
echo "fire" >> "`+fireCounter+`"
fires=$(grep -c fire "`+fireCounter+`")
if [ "$fires" -eq 1 ]; then
  echo "verify failed, retry" 1>&2
  exit 2
fi
exit 0`)

	subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
	out, code := driveAgentTool(t, dir, subScript, stopHook, nil)
	require.Equal(t, 0, code, "mock should exit 0 after the hook stops blocking; output:\n%s", out)

	assert.Equal(t, 2, runCount(t, counter),
		"exit-2 block on first SubagentStop must re-run the subagent exactly once (2 runs total)")

	// SubagentStop fired twice with the SAME agent_id (same subagent re-run, not a
	// new dispatch).
	ids := nonEmptyLines(readFile(t, idLog))
	require.Len(t, ids, 2, "SubagentStop must fire twice (block then allow)")
	assert.NotEmpty(t, ids[0], "agent_id must be present")
	assert.Equal(t, ids[0], ids[1], "re-run must reuse the SAME agent_id")
}

// TestT010_02_BlockViaDecisionJSONRerunsSubagent: a SubagentStop hook that exits 0
// with {"decision":"block","reason":"try again"} on its FIRST fire and prints
// nothing (clean exit 0) on its SECOND must also re-run the subagent. This is the
// path the REAL Claude SubagentStop contract uses (exit-0 + decision JSON, NOT a
// process error) and the one the a10n bridge handoff relies on — it MUST work.
//
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
func TestT010_02_BlockViaDecisionJSONRerunsSubagent(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "subagent-runs.txt")
	fireCounter := filepath.Join(dir, "stop-fires.txt")
	idLog := filepath.Join(dir, "stop-ids.txt")

	// First fire: exit 0 but emit a block decision on stdout. Later fires: clean.
	stopHook := writeHook(t, dir, "stop.sh", `input=$(cat)
echo "$input" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4 >> "`+idLog+`"
echo "fire" >> "`+fireCounter+`"
fires=$(grep -c fire "`+fireCounter+`")
if [ "$fires" -eq 1 ]; then
  printf '%s\n' '{"decision":"block","reason":"try again"}'
  exit 0
fi
exit 0`)

	subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
	out, code := driveAgentTool(t, dir, subScript, stopHook, nil)
	require.Equal(t, 0, code, "mock should exit 0 after the decision stops blocking; output:\n%s", out)

	assert.Equal(t, 2, runCount(t, counter),
		"exit-0 {decision:block} on first SubagentStop must re-run the subagent (2 runs total)")

	ids := nonEmptyLines(readFile(t, idLog))
	require.Len(t, ids, 2, "SubagentStop must fire twice (block-decision then clean)")
	assert.Equal(t, ids[0], ids[1], "re-run must reuse the SAME agent_id")
}

// TestT010_03_CleanStopRunsSubagentOnce: a SubagentStop hook that never blocks
// (always exit 0, no decision) must run the subagent exactly once — no re-run.
//
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
func TestT010_03_CleanStopRunsSubagentOnce(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "subagent-runs.txt")
	fireCounter := filepath.Join(dir, "stop-fires.txt")

	stopHook := writeHook(t, dir, "stop.sh", `cat >/dev/null
echo "fire" >> "`+fireCounter+`"
exit 0`)

	subScript := writeScript(t, dir, "sub.sh", countingSubagent(counter))
	out, code := driveAgentTool(t, dir, subScript, stopHook, nil)
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	assert.Equal(t, 1, runCount(t, counter), "a never-blocking SubagentStop must run the subagent exactly once")
	assert.Equal(t, 1, len(nonEmptyLines(readFile(t, fireCounter))), "SubagentStop must fire exactly once on a clean stop")
}

// TestT010_04_BlockCapBoundsTheLoop: a SubagentStop hook that ALWAYS blocks must
// be bounded by CLAUDE_CODE_STOP_HOOK_BLOCK_CAP. With the cap set to 2 the subagent
// runs 3 times (initial + 2 re-runs) then the mock gives up; with the default
// (unset = 8) it runs 9 times. The mock writes a "giving up" line to stderr.
//
// sr:docs https://code.claude.com/docs/en/env-vars
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
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
// Both blocking forms are covered in one run because they land by different
// routes inside emitStopHookAttachment (exit 2 arrives as a fire error, exit-0
// decision:block as out.Reason) and a fix could plausibly deliver one and drop
// the other.
//
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
func TestT010_07_BlockedSubagentStopSurfacesAsAttachment(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hookBody string
		want     string
	}{
		{
			name: "exit 2 with text on stderr",
			want: "verify failed: the delegated work is refused",
			hookBody: `cat >/dev/null
echo "verify failed: the delegated work is refused" 1>&2
exit 2`,
		},
		{
			name: "exit 0 with decision block",
			want: "a10n://check-runs/xyz needs resolving before this subagent may stop",
			hookBody: `cat >/dev/null
printf '%s' '{"decision":"block","reason":"a10n://check-runs/xyz needs resolving before this subagent may stop"}'
exit 0`,
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
			assert.Contains(t, transcript, `Stop hook feedback:\n`+tc.want,
				"the re-run sub-agent reads the refusal as a Stop hook feedback turn")
			assert.Contains(t, transcript, "hook_blocking_error",
				"a blocked SubagentStop must be recorded as a hook_blocking_error attachment, as a blocked Stop already is")
			assert.Contains(t, transcript, `"hookEvent":"SubagentStop"`,
				"the attachment must name SubagentStop, so a reader can tell which cycle refused")
			assert.Contains(t, transcript, tc.want,
				"the refusal's own text must reach the transcript — a block with no words is not a reportable refusal")
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

// readTranscript returns the contents of the DISPATCHING session's JSONL — the
// record the mock writes for a run at projDir with the given session id.
//
// Addressed rather than walked, and the session id alone is not enough to
// address it. A dispatch under isolation="worktree" leaves THREE .jsonl files:
// the parent's, the subagent's nested run under the WORKTREE's own encoded
// project directory — which shares the parent's session id and therefore its
// exact basename — and the subagent sidechain file. So neither "the last .jsonl
// the walk saw" nor "the one named <sessionID>.jsonl" names one conversation;
// both were tried and both read the subagent's two-line record, making a
// working mock look broken.
//
// What distinguishes them is the project directory each is filed under, which
// is the mock's own cwd encoding (sessionFilePath): symlinks resolved, then
// every non-alphanumeric byte replaced by a dash. Reproduced here because it is
// the only thing that separates the parent's record from a subagent's sharing
// its id.
func readTranscript(t *testing.T, configDir, projDir, sessionID string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(projDir)
	require.NoError(t, err, "resolve project dir")
	encoded := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(resolved, "-")
	path := filepath.Join(configDir, "projects", encoded, sessionID+".jsonl")
	require.FileExists(t, path, "the dispatching session's transcript must exist")
	return readFile(t, path)
}

// readSidechains concatenates every sub-agent transcript of a session.
func readSidechains(t *testing.T, configDir, projDir, sessionID string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(projDir)
	require.NoError(t, err, "resolve project dir")
	encoded := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(resolved, "-")
	paths, _ := filepath.Glob(filepath.Join(configDir, "projects", encoded, sessionID, "subagents", "agent-*.jsonl"))
	require.NotEmpty(t, paths, "the sub-agent's own transcript must exist")
	var all string
	for _, p := range paths {
		all += readFile(t, p)
	}
	return all
}

// --- shared helpers ---

// readFile reads a file the hook scripts wrote, failing the test if it is absent.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "expected file %s to exist", path)
	return string(data)
}

// nonEmptyLines splits on newlines and drops empty lines.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
