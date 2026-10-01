package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeHookSettings(t *testing.T, dir, event, hookPath string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"` + event + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hookPath + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
}

func captureHook(t *testing.T, dir, logFile, extract string) string {
	t.Helper()
	p := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\ninput=$(cat)\n"+extract+"\n"), 0o755))
	return p
}

// --- SubagentStart ---

// TestT004_01_NoSubagentHooksOnResume: a top-level --resume is the session
// going on, not a sub-agent: real Claude Code fires SessionStart (source
// resume), UserPromptSubmit, Stop and SessionEnd — no SubagentStart and no
// SubagentStop (a controlled claude 2.1.282 resume).
// sr:proves session-resume/claude
func TestT004_01_NoSubagentHooksOnResume(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := captureHook(t, dir, logFile,
		`evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
echo "$evt" >> "`+logFile+`"`)
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	h := `[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(
		`{"hooks":{"SessionStart":`+h+`,"UserPromptSubmit":`+h+`,"SubagentStart":`+h+`,"SubagentStop":`+h+`,"Stop":`+h+`,"SessionEnd":`+h+`}}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "sess-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	require.NoError(t, os.Remove(logFile))
	_, code = runInDir(t, dir, nil, "--script", script, "--resume", "sess-1", "--project-dir", dir, "-p", "again")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, "SessionStart\nUserPromptSubmit\nStop\nSessionEnd\n", string(data))
}

// TestT004_02_SubagentStartNotFiredForNewSession: --session-id must NOT fire SubagentStart.
func TestT004_02_SubagentStartNotFiredForNewSession(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := captureHook(t, dir, logFile,
		`echo "fired" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SubagentStart", hook)
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "sess-new", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	_, err := os.ReadFile(logFile)
	assert.True(t, os.IsNotExist(err), "SubagentStart must NOT fire for --session-id (new session)")
}

// TestT004_03_SubagentStartControlRecordOverridesAgentType: explicit control record sets agent_type.
// staged:proves subagent-lifecycle-hooks/claude
// sr:proves control-records
func TestT004_03_SubagentStartControlRecordOverridesAgentType(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := captureHook(t, dir, logFile,
		`agent=$(echo "$input" | grep -o '"agent_type":"[^"]*"' | cut -d'"' -f4)
echo "$agent" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SubagentStart", hook)
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"subagent_start","agent_type":"custom-agent"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "sess-new", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "custom-agent")
}

// TestT004_04_SubagentStartExit2DoesNotBlock: SubagentStart cannot block. An
// exit 2 is a non-blocking error recorded in the SUB-AGENT's own transcript,
// and the sub-agent runs (docs, "Exit code 2 behavior per event"; a controlled
// claude 2.1.282 run: hookName "SubagentStart:<agent_type>", stderr
// "[<command>]: <stderr>").
// sr:proves hook-exit-code-semantics/claude
// sr:proves hook-output-transcript-records/claude
// staged:proves subagent-lifecycle-hooks/claude
func TestT004_04_SubagentStartExit2DoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	blockHook := filepath.Join(dir, "block.sh")
	require.NoError(t, os.WriteFile(blockHook, []byte("#!/bin/sh\necho SAS-REFUSED >&2\nexit 2\n"), 0o755))
	writeHookSettings(t, dir, "SubagentStart", blockHook)
	sub := filepath.Join(dir, "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"SUB-RAN"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"SUB-RAN","is_error":false}'
`), 0o755))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if ! grep -q '"tool_use_id":"ag1"' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"ag1","name":"Agent","input":{"description":"d","prompt":"p","subagent_type":"worker","script":"`+sub+`"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "sess-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, out)
	matches, err := filepath.Glob(filepath.Join(dir, ".claude-config", "projects", "*", "sess-1", "subagents", "agent-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	side, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	assert.Contains(t, string(side), "SUB-RAN", "the sub-agent ran")
	assert.Contains(t, string(side), `"type":"hook_non_blocking_error"`)
	assert.Contains(t, string(side), `"hookName":"SubagentStart:worker"`)
	assert.Contains(t, string(side), `"stderr":"[`+blockHook+`]: SAS-REFUSED\n"`)
	mainFile, err := filepath.Glob(filepath.Join(dir, ".claude-config", "projects", "*", "sess-1.jsonl"))
	require.NoError(t, err)
	require.Len(t, mainFile, 1)
	mainData, err := os.ReadFile(mainFile[0])
	require.NoError(t, err)
	assert.NotContains(t, string(mainData), `"hookEvent":"SubagentStart"`, "the notice is not in the parent conversation")
}

// --- SubagentStop ---

// TestT004_05_SubagentStopNotFiredOnResume: an end_turn in a resumed
// top-level session fires Stop, never an agentless SubagentStop.
func TestT004_05_SubagentStopNotFiredOnResume(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := captureHook(t, dir, logFile, `echo fired >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SubagentStop", hook)
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "sess-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	_, code = runInDir(t, dir, nil, "--script", script, "--resume", "sess-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	_, err := os.ReadFile(logFile)
	assert.True(t, os.IsNotExist(err), "SubagentStop must not fire for a top-level session")
}

// TestT004_06_SubagentStopNotFiredForNewSession: SubagentStop must NOT fire for --session-id.
func TestT004_06_SubagentStopNotFiredForNewSession(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := captureHook(t, dir, logFile, `echo "fired" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SubagentStop", hook)
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "sess-new", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	_, err := os.ReadFile(logFile)
	assert.True(t, os.IsNotExist(err), "SubagentStop must NOT fire for --session-id")
}

// TestT004_07_SubagentPreToolUseCarriesAgentID: a Bash tool_use issued INSIDE a sub-agent
// fires PreToolUse with a NON-EMPTY agent_id, while the ROOT's own tool_use fires PreToolUse
// with an EMPTY agent_id. This is the real-claude contract a10n-workspace's hook depends on
// to tell a sub-agent's tool call from the root's (isSubagentPreTool keys on agent_id): a
// resolver sub-agent must be allowed to run `link resolve` while the root is denied. Without
// agent_id on the sub-agent's PreToolUse, the resolver is misclassified as the root, its
// `link resolve` is blocked, and it can never resolve its check (the e2e "got interrupted,
// want pass" failures that only surface when the published mock lacks this stamp).
// sr:proves hook-common-payload/claude
func TestT004_07_SubagentPreToolUseCarriesAgentID(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "pretool.log")
	// The hook records "<agent_id-or-EMPTY>" for every PreToolUse it sees.
	hook := captureHook(t, dir, logFile,
		`aid=$(echo "$input" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4)
echo "PTU:[${aid:-EMPTY}]" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "PreToolUse", hook)

	// The mock RE-RUNS each script once per turn, so a scenario must ADVANCE off the session
	// file (grep a prior tool_result) instead of re-emitting the same record — otherwise the
	// loop-guard aborts. Both scripts read $A10N_MOCK_SESSION_FILE for that.
	//
	// The SUB-agent's scenario: emit a Bash tool_use once, then a final result.
	subScript := filepath.Join(dir, "sub.sh")
	require.NoError(t, os.WriteFile(subScript, []byte(`#!/bin/sh
S=$(cat "${A10N_MOCK_SESSION_FILE:-/dev/null}" 2>/dev/null || true)
if ! printf '%s' "$S" | grep -q '"tool_use_id":"tu-sub"'; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-sub","name":"Bash","input":{"command":"true"}}]}}'
else
  printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
fi
`), 0o755))

	// The ROOT scenario: a Bash tool_use (turn 1) → spawn the sub-agent (turn 2, after the
	// root Bash tool_result is in the session) → final result (turn 3). We observe BOTH a root
	// PreToolUse (empty agent_id) and a sub-agent PreToolUse (non-empty agent_id) in one run.
	rootScript := filepath.Join(dir, "root.sh")
	require.NoError(t, os.WriteFile(rootScript, []byte(`#!/bin/sh
S=$(cat "${A10N_MOCK_SESSION_FILE:-/dev/null}" 2>/dev/null || true)
if ! printf '%s' "$S" | grep -q '"tool_use_id":"tu-root"'; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-root","name":"Bash","input":{"command":"true"}}]}}'
elif ! printf '%s' "$S" | grep -q '"tool_use_id":"tu-agent"'; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-agent","name":"Agent","input":{"subagent_type":"general-purpose","description":"work","prompt":"do work","script":"`+subScript+`"}}]}}'
else
  printf '%s\n' '{"type":"result","subtype":"success","result":"root done","is_error":false}'
fi
`), 0o755))

	_, code := runInDir(t, dir, nil, "--script", rootScript, "--session-id", "sess-ptu", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "PreToolUse must fire")
	got := string(data)
	// The root's Bash tool_use → PreToolUse with EMPTY agent_id.
	assert.Contains(t, got, "PTU:[EMPTY]", "root PreToolUse must carry NO agent_id\nlog:\n"+got)
	// The sub-agent's Bash tool_use → PreToolUse with a NON-EMPTY agent_id.
	assert.Regexp(t, `PTU:\[(?:agent-)?[^E].*\]`, got,
		"sub-agent PreToolUse must carry a non-empty agent_id\nlog:\n"+got)
}
