package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hookCapture(t *testing.T, dir, logFile, fields string) string {
	t.Helper()
	p := filepath.Join(dir, "hook.sh")
	// fields is a shell snippet that extracts what we want from $input
	require.NoError(t, os.WriteFile(p, []byte(`#!/bin/sh
input=$(cat)
`+fields+`
`), 0o755))
	return p
}

func writeHookSettings(t *testing.T, dir, event, hookPath string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"` + event + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hookPath + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
}

func writeScenario(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o755))
	return p
}

// --- SessionStart ---

// TestT003_01_SessionStartFiresOnNewSession: source must be "startup" for --session-id.
func TestT003_01_SessionStartFiresOnNewSession(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `
src=$(echo "$input" | grep -o '"source":"[^"]*"' | cut -d'"' -f4)
evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
echo "$evt:$src" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SessionStart", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-new", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, _ := os.ReadFile(logFile)
	assert.Contains(t, string(data), "SessionStart:startup")
}

// TestT003_02_SessionStartSourceResumeOnResume: source must be "resume" for --resume.
func TestT003_02_SessionStartSourceResumeOnResume(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `
src=$(echo "$input" | grep -o '"source":"[^"]*"' | cut -d'"' -f4)
echo "$src" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SessionStart", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--resume", "s-resume", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, _ := os.ReadFile(logFile)
	assert.Contains(t, string(data), "resume")
}

// TestT003_03_SessionStartBlockCausesNonZeroExit: SessionStart exit 2 must block.
func TestT003_03_SessionStartBlockCausesNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	hook := filepath.Join(dir, "block.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nexit 2\n"), 0o755))
	writeHookSettings(t, dir, "SessionStart", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code, "blocked SessionStart must exit non-zero")
}

// --- Stop ---

// TestT003_04_StopFiresWithEndTurnOnSuccess: stop_reason must be "end_turn" on clean run.
func TestT003_04_StopFiresWithEndTurnOnSuccess(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `
reason=$(echo "$input" | grep -o '"stop_reason":"[^"]*"' | cut -d'"' -f4)
echo "$reason" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "Stop", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "Stop hook must fire")
	assert.Contains(t, string(data), "end_turn")
}

// TestT003_05_StopFiresWithErrorOnScriptFailure: stop_reason must be "error" when script emits bad JSONL.
func TestT003_05_StopFiresWithErrorOnScriptFailure(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `
reason=$(echo "$input" | grep -o '"stop_reason":"[^"]*"' | cut -d'"' -f4)
echo "$reason" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "Stop", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' 'not-json-at-all'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "Stop hook must fire even on error")
	assert.Contains(t, string(data), "error")
}

// TestT003_06_SessionEndAlwaysFires: SessionEnd fires even when Stop is not configured.
func TestT003_06_SessionEndAlwaysFires(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `
evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
echo "$evt" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SessionEnd", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "SessionEnd hook must fire")
	assert.Contains(t, string(data), "SessionEnd")
}

// TestT003_07_StopFiresBeforeSessionEnd: both fire; Stop must come first (log order).
func TestT003_07_StopAndSessionEndBothFire(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `
evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
echo "$evt" >> "`+logFile+`"`)
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{
  "Stop":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}],
  "SessionEnd":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]
}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	lines := []string{}
	for _, l := range []string{"Stop", "SessionEnd"} {
		if assert.Contains(t, string(data), l) {
			_ = l
		}
	}
	_ = lines
	// Stop must appear before SessionEnd in the log.
	stopIdx := indexOf(string(data), "Stop")
	endIdx := indexOf(string(data), "SessionEnd")
	assert.Less(t, stopIdx, endIdx, "Stop must fire before SessionEnd")
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
