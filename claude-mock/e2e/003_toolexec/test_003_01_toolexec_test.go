package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT003_01_BashToolExecuted verifies that when the scenario script emits an
// assistant record with a Bash tool_use block, the mock executes the command and
// injects a user record with the tool_result into stdout.
func TestT003_01_BashToolExecuted(t *testing.T) {
	dir := t.TempDir()

	// Script emits one tool_use then (on second invocation, seeing session history)
	// emits the result frame.
	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
# On second invocation A10N_MOCK_SESSION_FILE will contain the tool_result.
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"got result","is_error":false}'
  exit 0
fi
# First invocation: emit a tool_use.
printf '%s\n' '{"type":"system","subtype":"init","session_id":"s1","tools":[]}'
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"echo hello-from-bash"}}]}}'
`), 0o755))

	out, code := runInDir(t, dir, nil,
		"--script", scriptPath,
		"--session-id", "s-bash",
		"--project-dir", dir,
		"--config-dir", dir,
		"-p", "go",
	)
	require.Equal(t, 0, code, "should exit 0; output:\n%s", out)

	// The assistant tool_use record must be in stdout.
	assert.Contains(t, out, `"tool_use"`, "assistant tool_use must be forwarded")
	// The synthesised tool_result must be in stdout.
	assert.Contains(t, out, `"tool_result"`, "synthesised tool_result must be in stdout")
	assert.Contains(t, out, "hello-from-bash", "bash output must appear in tool_result")
	// The result frame must be present.
	assert.Contains(t, out, `"type":"result"`, "result frame must be in stdout")
}

// TestT003_02_SessionFileContainsHistory verifies that A10N_MOCK_SESSION_FILE is
// written and contains the forwarded trajectory records after the run — but NOT the
// `result` frame, which real Claude Code streams to stdout and never persists to the
// transcript file (verified: 0 type:"result" records in a real transcript).
func TestT003_02_SessionFileContainsHistory(t *testing.T) {
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '%s\n' '{"type":"system","subtype":"init","session_id":"s2","tools":[]}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	configDir := filepath.Join(dir, "config")
	stdout, code := runInDir(t, dir, nil,
		"--script", scriptPath,
		"--session-id", "s2",
		"--project-dir", dir,
		"--config-dir", configDir,
		"-p", "go",
	)
	require.Equal(t, 0, code)

	// Find the session file under configDir/projects/...
	var sessionFiles []string
	filepath.Walk(configDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(p) == ".jsonl" {
			sessionFiles = append(sessionFiles, p)
		}
		return nil
	})
	require.Len(t, sessionFiles, 1, "exactly one session JSONL file must exist")

	data, err := os.ReadFile(sessionFiles[0])
	require.NoError(t, err)
	// The forwarded system record is persisted…
	assert.Contains(t, string(data), `"type":"system"`)
	// …the result frame is streamed to stdout…
	assert.Contains(t, stdout, `"type":"result"`, "the result frame must be streamed to stdout")
	// …but is NOT persisted to the transcript file, matching real Claude Code.
	assert.NotContains(t, string(data), `"type":"result"`,
		"real Claude Code never persists the result frame to the transcript file")
}

// TestT003_03_WriteToolCreatesFile verifies that the Write tool creates a file
// in the working directory and the script can observe it via A10N_MOCK_SESSION_FILE.
// staged:proves file-tools/claude
func TestT003_03_WriteToolCreatesFile(t *testing.T) {
	dir := t.TempDir()
	targetFile := filepath.Join(dir, "output.txt")

	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"file written","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"system","subtype":"init","session_id":"s3","tools":[]}'
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_2","name":"Write","input":{"file_path":"`+targetFile+`","content":"hello world"}}]}}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", scriptPath,
		"--session-id", "s3",
		"--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"),
		"-p", "go",
	)
	require.Equal(t, 0, code)

	data, err := os.ReadFile(targetFile)
	require.NoError(t, err, "Write tool must have created the file")
	assert.Equal(t, "hello world", string(data))
}
