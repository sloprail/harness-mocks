package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedReadIssues is the toolUseResult claude 2.1.285 recorded for a Read
// without file_path (recorded: snapshots/runs/tool-invalid-input).
const recordedReadIssues = `InputValidationError: [
  {
    "expected": "string",
    "code": "invalid_type",
    "path": [
      "file_path"
    ],
    "message": "Invalid input: expected string, received undefined"
  }
]`

// TestT017_29c_InvalidInputFiresNoHook: a call whose input the tool cannot
// take (a Read without file_path) is answered with a tool_use_error before any
// hook fires: no PreToolUse, no PostToolUse, no PostToolUseFailure; the tool
// does not run, and the turn goes on (recorded: snapshots/runs/tool-invalid-input).
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
// sr:proves tool-failure-hook/claude
// sr:proves agent-input-validation/claude
func TestT017_29c_InvalidInputFiresNoHook(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PreToolUse": h, "PostToolUse": h, "PostToolUseFailure": h})
	sc := script(t, dir, "s", toolUse("v1", "Read", `{"offset":1}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "vi-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "vi-1")), "v1turn-s-a")
	assert.Equal(t, "<tool_use_error>InputValidationError: Read failed due to the following issue:\nThe required parameter `file_path` is missing</tool_use_error>", block["content"])
	assert.Equal(t, true, block["is_error"])
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "vi-1"))
	require.NoError(t, err)
	want, _ := json.Marshal(recordedReadIssues)
	assert.Contains(t, string(raw), `"toolUseResult":`+string(want))
	_, err = os.Stat(log)
	assert.True(t, os.IsNotExist(err), "no hook fired")
	assert.Contains(t, out, `"result":"done"`, "the turn goes on")
	// The stream carries the same refusal as an error result, and the tool did
	// not run: the only tool_result is the validation error.
	frames := toolResultFrames(out)
	require.Len(t, frames, 1)
	assert.Equal(t, true, frames[0]["is_error"])
	assert.Contains(t, frames[0]["content"], "InputValidationError: Read failed")
}
