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

// TestT002_04_SubagentStartHookDoesNotFireOnResume: a top-level --resume is
// not a sub-agent — real Claude Code fires no SubagentStart for it (a
// controlled claude 2.1.282 resume fired SessionStart:resume,
// UserPromptSubmit, Stop and SessionEnd only).
func TestT002_04_SubagentStartHookDoesNotFireOnResume(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "subagent-log.txt")

	hookScript := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hookScript, []byte(`#!/bin/sh
cat >/dev/null
echo fired >> "`+logFile+`"
`), 0o755))

	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settings := `{"hooks":{"SubagentStart":[{"matcher":"*","hooks":[{"type":"command","command":"` + hookScript + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644))

	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil, "--session-id", "existing-session-id", "--script", scriptPath, "--project-dir", dir, "-p", "start")
	require.Equal(t, 0, code)
	_, code = runInDir(t, dir, nil, "--resume", "existing-session-id", "--script", scriptPath, "--project-dir", dir, "-p", "do work")
	require.Equal(t, 0, code)

	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "SubagentStart must not fire on a top-level --resume")
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

// TestT002_06_StopHookBlockReasonSurfacesAsAttachment verifies that a Stop hook returning
// exit-0 {"decision":"block","reason":...} (the shape an a10n drain emits to spawn resolver
// sub-agents) is SURFACED into the session transcript as a hook_blocking_error attachment
// carrying the reason — so a reactive agent can read the a10n://check-runs link and act on it.
// Before this, the mock discarded the Stop hook's output and the reason never reached the
// conversation, making reactive flows impossible to test.
func TestT002_06_StopHookBlockReasonSurfacesAsAttachment(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")

	// A Stop hook that emits the drain-shaped block: exit 0, decision:block + reason w/ a link.
	stopHook := filepath.Join(dir, "stop-block.sh")
	require.NoError(t, os.WriteFile(stopHook, []byte(`#!/bin/sh
printf '%s' '{"decision":"block","reason":"Checks need resolving. Spawn one sub-agent per link below; a10n://check-runs/abc/checks/inv/steps/review"}'
exit 0
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
		"--session-id", "s-stopblock",
		"--project-dir", dir,
		"--config-dir", configDir,
		"-p", "run",
	)
	require.Equal(t, 0, code)

	// The Stop hook's block reason (with the a10n://check-runs link) must be recorded in the
	// session transcript as an attachment, so a reactive agent turn can read + act on it.
	var sessionFile string
	filepath.Walk(configDir, func(p string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() && filepath.Ext(p) == ".jsonl" {
			sessionFile = p
		}
		return nil
	})
	require.NotEmpty(t, sessionFile, "session transcript must exist")
	data, err := os.ReadFile(sessionFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "hook_blocking_error",
		"Stop hook block must be recorded as a hook_blocking_error attachment")
	assert.Contains(t, string(data), "a10n://check-runs/abc/checks/inv/steps/review",
		"the block reason's link must reach the transcript so a reactive agent can read it")
}

// TestT002_07_StopBlockRePromptsTurnSameRun verifies the real Claude Code Stop→re-prompt→continue
// loop: a Stop hook that blocks ONCE re-prompts the agent, and the scenario's NEXT turn fires in
// the SAME run (reacting to the surfaced block) — rather than the run ending at the first result.
// This is what lets a reactive scenario spawn a resolver after the Stop drain parks a check.
func TestT002_07_StopBlockRePromptsTurnSameRun(t *testing.T) {
	dir := t.TempDir()

	// Stop hook: block ONCE (until a marker file exists), then allow — so the loop is bounded.
	gate := filepath.Join(dir, "stopped-once")
	stopHook := filepath.Join(dir, "stop.sh")
	require.NoError(t, os.WriteFile(stopHook, []byte(`#!/bin/sh
if [ -f "`+gate+`" ]; then exit 0; fi
touch "`+gate+`"
printf '%s' '{"decision":"block","reason":"keep going: a10n://check-runs/x/checks/c/steps/s"}'
exit 0
`), 0o755))

	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{
  "hooks": { "Stop": [ {"matcher":"*","hooks":[{"type":"command","command":"`+stopHook+`"}]} ] }
}`), 0o644))

	// The scenario: turn 1 emits a result (ends turn → Stop fires + blocks). On the re-prompt,
	// turn 2 (gated on the surfaced block link being in $SESS) runs a Bash command that proves it
	// fired in the same run, then a final result.
	proof := filepath.Join(dir, "second-turn-ran")
	scriptPath := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
SF="${A10N_MOCK_SESSION_FILE:-/dev/null}"
SESS=$(cat "$SF" 2>/dev/null || true)
# turn 2: if the Stop block's link surfaced, run a Bash tool_use to prove we continued.
if printf '%s' "$SESS" | grep -q 'a10n://check-runs/x/checks/c/steps/s'; then
  if ! printf '%s' "$SESS" | grep -q 'second-turn-marker'; then
    printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"second-turn-marker","name":"Bash","input":{"command":"touch `+proof+`"}}]}}'
    exit 0
  fi
fi
# turn 1 (and final): end the turn.
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", scriptPath, "--session-id", "s-reprompt", "--project-dir", dir, "-p", "run",
	)
	require.Equal(t, 0, code)

	// The second turn ran IN THE SAME RUN because the Stop block re-prompted the agent.
	_, err := os.Stat(proof)
	assert.NoError(t, err, "Stop-block re-prompt must let the next scenario turn fire in the same run")
}
