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
// staged:proves hook-exit-code-semantics/claude
// staged:proves subagent-stop-block-loop/claude
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
// staged:proves subagent-stop-block-loop/claude
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
