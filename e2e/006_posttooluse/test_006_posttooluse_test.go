package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSettings(t *testing.T, dir, event, hookPath string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"` + event + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hookPath + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
}

// TestT006_01_PostToolUseFiresAfterSynthesisedToolResult: PostToolUse fires after mock executes a real tool.
func TestT006_01_PostToolUseFiresAfterSynthesisedToolResult(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
tool=$(echo "$input" | grep -o '"tool_name":"[^"]*"' | cut -d'"' -f4)
echo "$tool" >> "`+logFile+`"
`), 0o755))
	writeSettings(t, dir, "PostToolUse", hook)

	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"echo post"}}]}}'
`), 0o755))

	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "PostToolUse hook must fire")
	assert.Contains(t, string(data), "Bash")
}

// TestT006_02_PostToolUseFiresForInlineToolResult: PostToolUse fires for standalone user tool_result records.
// Scripts that pre-compute tool results can emit a user record with tool_result directly (no preceding
// assistant tool_use in the same turn). The mock fires PostToolUse when it sees any user/tool_result block.
func TestT006_02_PostToolUseFiresForInlineToolResult(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
tool=$(echo "$input" | grep -o '"tool_name":"[^"]*"' | cut -d'"' -f4)
echo "$tool" >> "`+logFile+`"
`), 0o755))
	writeSettings(t, dir, "PostToolUse", hook)

	// Script emits only a user record with tool_result (no assistant tool_use in this turn).
	// This simulates a script that provides pre-computed tool results.
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_1","name":"Read","content":"file content"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "PostToolUse hook must fire for inline tool_result")
	assert.Contains(t, string(data), "Read")
}

// TestT006_03_PostToolUseNonBlockingOnExitOne: PostToolUse exit 1 is non-blocking (run completes).
func TestT006_03_PostToolUseNonBlockingOnExitOne(t *testing.T) {
	dir := t.TempDir()
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755))
	writeSettings(t, dir, "PostToolUse", hook)

	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_1","name":"Bash","content":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	assert.Equal(t, 0, code, "PostToolUse exit 1 must be non-blocking")
}
