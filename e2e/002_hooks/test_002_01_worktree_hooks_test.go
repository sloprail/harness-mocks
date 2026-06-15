package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT002_01_WorktreeCreateHookFires verifies that a script emitting a
// worktree_create control record causes the configured WorktreeCreate command
// hook to fire. The hook writes the worktree name to a temp file; we assert
// the file contains the expected name.
func TestT002_01_WorktreeCreateHookFires(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "hook-log.txt")

	// Write the hook script that records the worktree name from its stdin.
	hookScript := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hookScript, []byte(`#!/bin/sh
# Read hook input from stdin and write the worktree_name field to logFile.
input=$(cat)
name=$(echo "$input" | grep -o '"worktree_name":"[^"]*"' | cut -d'"' -f4)
echo "$name" >> "`+logFile+`"
`), 0o755))

	// Write .claude/settings.json configuring the WorktreeCreate command hook.
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settings := `{
  "hooks": {
    "WorktreeCreate": [
      {
        "matcher": "*",
        "hooks": [
          {"type": "command", "command": "` + hookScript + `"}
        ]
      }
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644))

	// Write the mock scenario script.
	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/my-feature"}'
printf '%s\n' '{"type":"system","subtype":"init","session_id":"s1","tools":[]}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	out, code := runInDir(t, dir, nil,
		"--script", scriptPath,
		"--session-id", "s1",
		"--project-dir", dir,
		"-p", "go",
	)
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// The worktree_create record must NOT appear in stdout (it's a control record).
	assert.NotContains(t, out, "worktree_create", "control record must not be forwarded to stdout")

	// The hook must have recorded the worktree name.
	logBytes, err := os.ReadFile(logFile)
	require.NoError(t, err, "hook log file must exist — hook did not fire")
	assert.Contains(t, string(logBytes), "feat/my-feature", "hook must receive the worktree name")
}

// TestT002_02_WorktreeCreateHookBlockExitsNonZero verifies that a WorktreeCreate
// hook that exits 2 causes the mock to exit non-zero.
func TestT002_02_WorktreeCreateHookBlockExitsNonZero(t *testing.T) {
	dir := t.TempDir()

	blockScript := filepath.Join(dir, "block.sh")
	require.NoError(t, os.WriteFile(blockScript, []byte(`#!/bin/sh
echo "worktree creation blocked by policy" >&2
exit 2
`), 0o755))

	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settings := `{
  "hooks": {
    "WorktreeCreate": [
      {"matcher":"*","hooks":[{"type":"command","command":"` + blockScript + `"}]}
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644))

	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '%s\n' '{"type":"worktree_create","worktree_name":"feat/blocked"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"should not reach","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", scriptPath,
		"--session-id", "s-block",
		"--project-dir", dir,
		"-p", "go",
	)
	assert.NotEqual(t, 0, code, "blocked worktree hook must cause non-zero exit")
}

// TestT002_04_SubagentStartHookFiresOnResume verifies that SubagentStart fires
// automatically when the mock is invoked with --resume (subagent session).
func TestT002_04_SubagentStartHookFiresOnResume(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "subagent-log.txt")

	hookScript := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hookScript, []byte(`#!/bin/sh
input=$(cat)
event=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
agent=$(echo "$input" | grep -o '"agent_type":"[^"]*"' | cut -d'"' -f4)
echo "$event:$agent" >> "`+logFile+`"
`), 0o755))

	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settings := `{
  "hooks": {
    "SubagentStart": [
      {"matcher":"*","hooks":[{"type":"command","command":"` + hookScript + `"}]}
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644))

	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--resume", "existing-session-id",
		"--script", scriptPath,
		"--project-dir", dir,
		"-p", "do work",
	)
	require.Equal(t, 0, code)

	logBytes, err := os.ReadFile(logFile)
	require.NoError(t, err, "SubagentStart hook log must exist")
	assert.Contains(t, string(logBytes), "SubagentStart:general-purpose",
		"SubagentStart must fire with agent_type=general-purpose on --resume")
}

// TestT002_05_SubagentStartControlRecordOverridesAgentType verifies that a
// subagent_start control record in the script fires SubagentStart with the
// agent_type from the record, overriding the default.
func TestT002_05_SubagentStartControlRecordOverridesAgentType(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "subagent-log.txt")

	hookScript := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hookScript, []byte(`#!/bin/sh
input=$(cat)
agent=$(echo "$input" | grep -o '"agent_type":"[^"]*"' | cut -d'"' -f4)
echo "$agent" >> "`+logFile+`"
`), 0o755))

	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settings := `{
  "hooks": {
    "SubagentStart": [
      {"matcher":"*","hooks":[{"type":"command","command":"` + hookScript + `"}]}
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644))

	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '%s\n' '{"type":"subagent_start","agent_type":"custom-agent"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--session-id", "new-session",
		"--script", scriptPath,
		"--project-dir", dir,
		"-p", "go",
	)
	require.Equal(t, 0, code)

	logBytes, err := os.ReadFile(logFile)
	require.NoError(t, err, "hook log must exist")
	assert.Contains(t, string(logBytes), "custom-agent",
		"subagent_start control record must fire hook with its agent_type")
}

// TestT002_03_StopHookFires verifies that the Stop hook fires after the mock
// script completes (on the result frame).
func TestT002_03_StopHookFires(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "stop-log.txt")

	stopHook := filepath.Join(dir, "stop.sh")
	require.NoError(t, os.WriteFile(stopHook, []byte(`#!/bin/sh
input=$(cat)
reason=$(echo "$input" | grep -o '"stop_reason":"[^"]*"' | cut -d'"' -f4)
echo "stop:$reason" >> "`+logFile+`"
`), 0o755))

	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settings := `{
  "hooks": {
    "Stop": [
      {"matcher":"*","hooks":[{"type":"command","command":"` + stopHook + `"}]}
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644))

	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", scriptPath,
		"--session-id", "s-stop",
		"--project-dir", dir,
		"-p", "run",
	)
	require.Equal(t, 0, code)

	logBytes, err := os.ReadFile(logFile)
	require.NoError(t, err, "stop hook log must exist")
	assert.Contains(t, string(logBytes), "stop:", "stop hook must fire with stop_reason")
}
