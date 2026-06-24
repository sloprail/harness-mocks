package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeHookSettings(t *testing.T, dir, event, matcher, hookPath string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"` + event + `":[{"matcher":"` + matcher + `","hooks":[{"type":"command","command":"` + hookPath + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
}

// assistantWithTool returns a minimal assistant record containing one tool_use block.
func assistantWithTool(toolName, toolUseID string) string {
	return `{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"` + toolUseID + `","name":"` + toolName + `","input":{"command":"echo hi"}}]}}`
}

// userWithToolResult returns a minimal user record containing a tool_result block for inline static scripts.
func userWithToolResult(toolUseID, toolName string) string {
	return `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + toolUseID + `","name":"` + toolName + `","content":"ok"}]}}`
}

// TestT005_01_PreToolUseHookFires: hook fires before tool_use is forwarded.
func TestT005_01_PreToolUseHookFires(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
tool=$(echo "$input" | grep -o '"tool_name":"[^"]*"' | cut -d'"' -f4)
echo "$tool" >> "`+logFile+`"
`), 0o755))
	writeHookSettings(t, dir, "PreToolUse", "*", hook)

	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '`+assistantWithTool("Bash", "tu_1")+`'
`), 0o755))

	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "PreToolUse hook must have fired")
	assert.Contains(t, string(data), "Bash")
}

// TestT005_02_PreToolUseBlockExitsNonZero: hook exit 2 must block tool execution and exit non-zero.
func TestT005_02_PreToolUseBlockExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	blockHook := filepath.Join(dir, "block.sh")
	require.NoError(t, os.WriteFile(blockHook, []byte("#!/bin/sh\necho 'blocked' >&2\nexit 2\n"), 0o755))
	writeHookSettings(t, dir, "PreToolUse", "*", blockHook)

	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '`+assistantWithTool("Bash", "tu_1")+`'
printf '%s\n' '{"type":"result","subtype":"success","result":"should not reach","is_error":false}'
`), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	assert.NotEqual(t, 0, code, "blocked PreToolUse must cause non-zero exit; output:\n%s", out)
}

// TestT005_03_PreToolUseMatcherFiltersToolName: hook with specific matcher only fires for that tool.
func TestT005_03_PreToolUseMatcherFiltersToolName(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
echo "fired" >> "`+logFile+`"
`), 0o755))
	// Only fires for "Write", not for "Bash".
	writeHookSettings(t, dir, "PreToolUse", "Write", hook)

	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '`+assistantWithTool("Bash", "tu_1")+`'
`), 0o755))

	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code)
	_, err := os.ReadFile(logFile)
	assert.True(t, os.IsNotExist(err), "PreToolUse hook with matcher=Write must NOT fire for Bash")
}

// TestT005_05_PreToolUseDenyBlocksAndAgentRetries pins the real Claude Code contract
// (verified empirically against the real binary): a PreToolUse hook returning
// permissionDecision=deny with EXIT 0 BLOCKS the tool call and feeds the reason back
// to the agent — the turn CONTINUES (it is NOT aborted), so the agent can self-correct
// and RETRY. With a broad matcher "*" the hook fires for a normal Bash call.
//
// The hook denies the FIRST Bash call (deny-once via a counter file) and allows the
// rest. The script emits a Bash tool_use; on its next turn it detects the blocked
// tool_result (is_error + the deny reason) in the session and RETRIES with another
// Bash tool_use, which is allowed and executed. The final result proves the run did
// not abort.
func TestT005_05_PreToolUseDenyBlocksAndAgentRetries(t *testing.T) {
	dir := t.TempDir()
	cntFile := filepath.Join(dir, "deny_count")
	hook := filepath.Join(dir, "deny_once.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
tool=$(printf '%s' "$input" | grep -o '"tool_name":"[^"]*"' | head -1 | cut -d'"' -f4)
[ "$tool" = "Bash" ] || exit 0
n=$(cat `+cntFile+` 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > `+cntFile+`
if [ "$n" -le 1 ]; then
  printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"DENY_THEN_RETRY: do not run that; retry the Bash call."}}'
fi
exit 0
`), 0o755))
	writeHookSettings(t, dir, "PreToolUse", "*", hook)

	// Turn 1: emit a Bash tool_use. Turn 2+: if the session shows the blocked
	// tool_result (is_error true) and we have not yet retried, emit a SECOND Bash
	// tool_use (the retry). Once a successful (non-error) tool_result exists, finish.
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
SF="$A10N_MOCK_SESSION_FILE"
if [ -n "$SF" ] && grep -q '"is_error":false' "$SF" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
if [ -n "$SF" ] && grep -q 'DENY_THEN_RETRY' "$SF" 2>/dev/null; then
  # The first call was blocked + the reason fed back — RETRY.
  printf '%s\n' '`+assistantWithTool("Bash", "tu_2")+`'
  exit 0
fi
printf '%s\n' '`+assistantWithTool("Bash", "tu_1")+`'
`), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equalf(t, 0, code, "run must complete (deny blocks the tool but does NOT abort the turn); output:\n%s", out)

	// The hook fired at least twice on Bash (first denied, retry allowed).
	cnt := strings.TrimSpace(readFileOr(cntFile))
	assert.NotEqual(t, "0", cnt, "PreToolUse must have fired on Bash")
	assert.NotEqual(t, "1", cnt, "the agent must have RETRIED the blocked Bash call (>=2 fires); got %s", cnt)

	// The blocked tool_result (with the deny reason) was fed back to the agent.
	assert.Contains(t, out, "DENY_THEN_RETRY", "the deny reason must be surfaced to the agent as a tool_result")
	assert.Contains(t, out, "blocked by a PreToolUse hook", "the blocked tool_result must mark the block")
}

// readFileOr returns the file contents or "" when unreadable.
func readFileOr(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// TestT005_04_PreToolUseReceivesToolInput: hook stdin contains tool_input JSON.
func TestT005_04_PreToolUseReceivesToolInput(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
# tool_input should be present as a JSON field
echo "$input" >> "`+logFile+`"
`), 0o755))
	writeHookSettings(t, dir, "PreToolUse", "*", hook)

	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '`+assistantWithTool("Bash", "tu_1")+`'
`), 0o755))

	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "tool_input")
	assert.Contains(t, string(data), "echo hi")
}
