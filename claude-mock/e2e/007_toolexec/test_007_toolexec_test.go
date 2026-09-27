package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolUseScript returns a scenario script that emits one tool_use on the first
// invocation, then a result frame once the session file contains tool_result.
func toolUseScript(t *testing.T, dir, toolName, inputJSON string) string {
	t.Helper()
	p := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(p, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_1","name":"`+toolName+`","input":`+inputJSON+`}]}}'
`), 0o755))
	return p
}

func runTool(t *testing.T, dir, toolName, inputJSON string) (out string, code int) {
	t.Helper()
	script := toolUseScript(t, dir, toolName, inputJSON)
	return runInDir(t, dir, nil,
		"--script", script,
		"--session-id", "s1",
		"--project-dir", dir,
		"--config-dir", filepath.Join(dir, "cfg"),
		"-p", "go",
	)
}

// --- Bash ---

// TestT007_01_BashToolExecutesCommand: Bash tool runs the command and returns output.
func TestT007_01_BashToolExecutesCommand(t *testing.T) {
	dir := t.TempDir()
	out, code := runTool(t, dir, "Bash", `{"command":"echo hello-bash"}`)
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Contains(t, out, "hello-bash")
	assert.Contains(t, out, `"tool_result"`)
}

// TestT007_02_BashToolFailureResultIsError: failing command sets is_error=true in tool_result.
func TestT007_02_BashToolFailureResultIsError(t *testing.T) {
	dir := t.TempDir()
	out, code := runTool(t, dir, "Bash", `{"command":"exit 1"}`)
	require.Equal(t, 0, code, "mock itself must succeed even when bash fails; output:\n%s", out)
	assert.Contains(t, out, `"is_error":true`)
}

// TestT007_03_BashToolCwdIsProjectDir: Bash executes relative to the project cwd.
func TestT007_03_BashToolCwdIsProjectDir(t *testing.T) {
	dir := t.TempDir()
	// Create a sentinel file; bash pwd should print dir.
	out, code := runTool(t, dir, "Bash", `{"command":"pwd"}`)
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Contains(t, out, dir)
}

// TestT007_11_BashToolSeesMockSessionID: a Bash tool subprocess sees the mock's
// --session-id as CLAUDE_CODE_SESSION_ID, as real Claude Code exports it into every
// Bash tool subprocess — even when the mock's own environment carries a DIFFERENT
// value (the operator's outer session, when tests run inside a live Claude Code
// session). Without it, a command resolving "the current session" from
// CLAUDE_CODE_SESSION_ID (e.g. `sr-session trajectory cite`) finds no transcript in
// CI, or silently resolves against the operator's session.
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID)
func TestT007_11_BashToolSeesMockSessionID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "decoy-outer-session")
	logPath := filepath.Join(dir, "sid.log")
	out, code := runTool(t, dir, "Bash", `{"command":"printf %s \"$CLAUDE_CODE_SESSION_ID\" > `+logPath+`"}`)
	require.Equal(t, 0, code, "output:\n%s", out)
	got, err := os.ReadFile(logPath)
	require.NoError(t, err, "bash command must have run; output:\n%s", out)
	assert.Equal(t, "s1", string(got), "Bash tool must see the mock's --session-id, not the inherited decoy")
}

// --- Read ---

// TestT007_04_ReadToolReturnsFileContent: Read returns the file contents.
func TestT007_04_ReadToolReturnsFileContent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(f, []byte("line1\nline2\n"), 0o644))
	out, code := runTool(t, dir, "Read", `{"file_path":"`+f+`"}`)
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Contains(t, out, "line1")
	assert.Contains(t, out, "line2")
}

// TestT007_05_ReadToolMissingFileIsError: Read on non-existent file sets is_error=true.
func TestT007_05_ReadToolMissingFileIsError(t *testing.T) {
	dir := t.TempDir()
	out, code := runTool(t, dir, "Read", `{"file_path":"/nonexistent/file.txt"}`)
	require.Equal(t, 0, code)
	assert.Contains(t, out, `"is_error":true`)
}

// --- Write ---

// TestT007_06_WriteToolCreatesFile: Write creates the file with correct content.
func TestT007_06_WriteToolCreatesFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	out, code := runTool(t, dir, "Write", `{"file_path":"`+target+`","content":"written by mock"}`)
	require.Equal(t, 0, code, "output:\n%s", out)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "written by mock", string(data))
}

// TestT007_07_WriteToolCreatesParentDirs: Write creates missing parent directories.
func TestT007_07_WriteToolCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c.txt")
	out, code := runTool(t, dir, "Write", `{"file_path":"`+target+`","content":"nested"}`)
	require.Equal(t, 0, code, "output:\n%s", out)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

// --- Edit ---

// TestT007_08_EditToolReplacesContent: Edit replaces old_string with new_string.
func TestT007_08_EditToolReplacesContent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(f, []byte("hello world"), 0o644))
	out, code := runTool(t, dir, "Edit", `{"file_path":"`+f+`","old_string":"world","new_string":"mock"}`)
	require.Equal(t, 0, code, "output:\n%s", out)
	data, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Equal(t, "hello mock", string(data))
}

// TestT007_09_EditToolOldStringNotFoundIsError: Edit returns is_error when old_string not found.
func TestT007_09_EditToolOldStringNotFoundIsError(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(f, []byte("hello world"), 0o644))
	out, code := runTool(t, dir, "Edit", `{"file_path":"`+f+`","old_string":"nothere","new_string":"x"}`)
	require.Equal(t, 0, code)
	assert.Contains(t, out, `"is_error":true`)
}

// --- Unknown tool ---

// TestT007_10_UnknownToolReturnsError: unknown tool name produces is_error=true result.
func TestT007_10_UnknownToolReturnsError(t *testing.T) {
	dir := t.TempDir()
	out, code := runTool(t, dir, "FlyingUnicorn", `{}`)
	require.Equal(t, 0, code, "mock must not crash on unknown tool; output:\n%s", out)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, "not implemented")
}
