package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
// The real Claude Code SessionStart payload carries "source"
// ("startup"|"resume"|"clear"|"compact") — verified empirically against claude 2.x;
// it is NOT "trigger". The mock mirrors this — hooks must read the "source" field.
func TestT003_01_SessionStartFiresOnNewSession(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `
trig=$(echo "$input" | grep -o '"source":"[^"]*"' | cut -d'"' -f4)
evt=$(echo "$input" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
echo "$evt:$trig" >> "`+logFile+`"`)
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
trig=$(echo "$input" | grep -o '"source":"[^"]*"' | cut -d'"' -f4)
echo "$trig" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "SessionStart", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-resume", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	_, code = runInDir(t, dir, nil, "--script", script, "--resume", "s-resume", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, _ := os.ReadFile(logFile)
	assert.Contains(t, string(data), "resume")
}

// TestT003_03_SessionStartExit2DoesNotBlock: SessionStart cannot block. An
// exit 2 is recorded as a non-blocking error — stderr quoted as
// "[<command>]: <stderr>" — and the session runs (docs, "Exit code 2 behavior
// per event"; a controlled claude 2.1.282 run).
func TestT003_03_SessionStartExit2DoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	hook := filepath.Join(dir, "block.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho SS-REFUSED >&2\nexit 2\n"), 0o755))
	writeHookSettings(t, dir, "SessionStart", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ran"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "a SessionStart exit 2 does not stop the session: %s", out)
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	enc := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(resolved, "-")
	data, err := os.ReadFile(filepath.Join(cfg, "projects", enc, "s1.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var att map[string]any
	for _, l := range lines {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &rec))
		if a, ok := rec["attachment"].(map[string]any); ok && a["hookEvent"] == "SessionStart" {
			att = a
			assert.Nil(t, rec["parentUuid"], "written first, the notice is the transcript's origin")
		}
	}
	require.NotNil(t, att, "the exit 2 is recorded")
	assert.Equal(t, "hook_non_blocking_error", att["type"])
	assert.Equal(t, "SessionStart:startup", att["hookName"])
	assert.Equal(t, "["+hook+"]: SS-REFUSED\n", att["stderr"])
	assert.EqualValues(t, 2, att["exitCode"])
	assert.Equal(t, hook, att["command"])
	assert.NotContains(t, att, "durationMs", "claude 2.1.282 records no duration for it")
	assert.Contains(t, string(data), `"text":"ran"`, "the turn ran")
}

// --- Stop ---

// TestT003_04_StopPayloadIsTheRealOne: Stop carries stop_hook_active (false
// included), last_assistant_message, background_tasks and session_crons — and
// no stop_reason, which real Claude Code does not send (docs, Stop input; a
// claude 2.1.282 payload).
func TestT003_04_StopPayloadIsTheRealOne(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `printf '%s\n' "$input" >> "`+logFile+`"`)
	writeHookSettings(t, dir, "Stop", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}'
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"all done here"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code)
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "Stop hook must fire")
	var p map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(data))), &p))
	assert.Equal(t, false, p["stop_hook_active"])
	assert.Equal(t, "all done here", p["last_assistant_message"])
	assert.Equal(t, []any{}, p["background_tasks"])
	assert.Equal(t, []any{}, p["session_crons"])
	assert.NotContains(t, p, "stop_reason")
}

// TestT003_05_NoStopWhenTheScriptFails: a run whose script fails ends without
// Stop — real Claude Code fires Stop when the agent finishes responding, and
// StopFailure (not modelled) for API errors.
func TestT003_05_NoStopWhenTheScriptFails(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.txt")
	hook := hookCapture(t, dir, logFile, `echo fired >> "`+logFile+`"`)
	writeHookSettings(t, dir, "Stop", hook)
	script := writeScenario(t, dir, `#!/bin/sh
printf '%s\n' 'not-json-at-all'
`)
	_, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code)
	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "Stop must not fire for a failed run")
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
