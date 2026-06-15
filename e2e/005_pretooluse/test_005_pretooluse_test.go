package e2e

import (
	"os"
	"path/filepath"
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
