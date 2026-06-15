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

// TestT004_01_SubagentStartFiresOnResume: --resume triggers SubagentStart with agent_type=general-purpose.
func TestT004_01_SubagentStartFiresOnResume(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := captureHook(t, dir, logFile,
		`evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
agent=$(echo "$input" | grep -o '"agent_type":"[^"]*"' | cut -d'"' -f4)
echo "$evt:$agent" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SubagentStart", hook)
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--resume", "sess-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "SubagentStart hook must fire")
	assert.Contains(t, string(data), "SubagentStart:general-purpose")
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

// TestT004_04_SubagentStartBlockCausesNonZeroExit: SubagentStart exit 2 must block.
func TestT004_04_SubagentStartBlockCausesNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	blockHook := filepath.Join(dir, "block.sh")
	require.NoError(t, os.WriteFile(blockHook, []byte("#!/bin/sh\nexit 2\n"), 0o755))
	writeHookSettings(t, dir, "SubagentStart", blockHook)
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--resume", "sess-1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code, "blocked SubagentStart must exit non-zero")
}

// --- SubagentStop ---

// TestT004_05_SubagentStopFiresOnEndTurnForResume: SubagentStop fires when assistant emits end_turn in --resume session.
func TestT004_05_SubagentStopFiresOnEndTurnForResume(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := captureHook(t, dir, logFile,
		`evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
echo "$evt" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SubagentStop", hook)
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	_, code := runInDir(t, dir, nil, "--script", script, "--resume", "sess-1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "SubagentStop hook must fire")
	assert.Contains(t, string(data), "SubagentStop")
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
