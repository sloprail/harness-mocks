package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const resultFrame = `{"type":"result","subtype":"success","result":"done","is_error":false}`

// exitHookScript is a hook that drains stdin, writes msg to stderr and/or
// stdout, and exits with code.
func exitHookScript(stdout, stderr string, code int) string {
	b := "cat >/dev/null\n"
	if stdout != "" {
		b += "printf '%s\\n' '" + stdout + "'\n"
	}
	if stderr != "" {
		b += "printf '%s\\n' '" + stderr + "' >&2\n"
	}
	return b + "exit " + string(rune('0'+code%10)) + "\n"
}

// toolScenario runs a Bash tool_use that writes toolFile on its first run and
// finishes on its second. Every run appends a line to runsLog and snapshots the
// session file (what the "model" sees) to sessionCopy.
func toolScenario(t *testing.T, dir, runsLog, sessionCopy, toolFile string) string {
	return toolScenarioCmd(t, dir, runsLog, sessionCopy, "echo ran > "+toolFile)
}

// toolScenarioCmd is toolScenario with an arbitrary Bash command.
func toolScenarioCmd(t *testing.T, dir, runsLog, sessionCopy, cmd string) string {
	return writeScript(t, dir, "scn.sh", `#!/bin/sh
echo run >> "`+runsLog+`"
cp "$A10N_MOCK_SESSION_FILE" "`+sessionCopy+`" 2>/dev/null
if [ "$(wc -l < "`+runsLog+`")" -ge 2 ]; then
  printf '%s\n' '`+resultFrame+`'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_x1","name":"Bash","input":{"command":"`+cmd+`"}}]}}'
`)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func readOrEmpty(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_01_UserPromptSubmitExit2BlocksPrompt(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "scenario-ran")
	writeSettings(t, dir, map[string]string{
		"UserPromptSubmit": writeHook(t, dir, "ups.sh", exitHookScript("", "prompt refused", 2)),
	})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\necho x > \""+marker+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-b", "--project-dir", dir, "-p", "hi")
	t.Logf("mock exit=%d output:\n%s", code, out)
	assert.False(t, fileExists(marker), "a blocked prompt never reaches the model: the scenario must not run")
	assert.NotEqual(t, 0, code, "a blocked prompt is a failed run")
	assert.Contains(t, out, "prompt refused", "the block reason (stderr) is shown")
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_02_PlainTextStdoutExit0IsContext(t *testing.T) {
	for _, event := range []string{"UserPromptSubmit", "SessionStart"} {
		t.Run(event, func(t *testing.T) {
			dir := t.TempDir()
			ctxLog := filepath.Join(dir, "ctx.log")
			writeSettings(t, dir, map[string]string{
				event: writeHook(t, dir, "h.sh", exitHookScript("The secret word is BANANA.", "", 0)),
			})
			script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf %s \"$A10N_MOCK_ADDITIONAL_CONTEXT\" > \""+ctxLog+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-c", "--project-dir", dir, "-p", "hi")
			require.Equal(t, 0, code, "output:\n%s", out)
			assert.Contains(t, readOrEmpty(ctxLog), "The secret word is BANANA.")
		})
	}
}

// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_03_PreToolUseNonBlockingExitCodesLetToolRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		path bool // hook path does not exist
	}{{"exit 1", 1, false}, {"exit 127 missing command", 127, true}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			toolFile := filepath.Join(dir, "tool-ran")
			hook := filepath.Join(dir, "does-not-exist.sh")
			if !tc.path {
				hook = writeHook(t, dir, "pre.sh", exitHookScript("", "pre failure", tc.code))
			}
			writeSettings(t, dir, map[string]string{"PreToolUse": hook})
			script := toolScenario(t, dir, filepath.Join(dir, "runs"), filepath.Join(dir, "sess"), toolFile)
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-p1", "--project-dir", dir, "-p", "go")
			require.Equal(t, 0, code, "output:\n%s", out)
			assert.True(t, fileExists(toolFile), "a non-blocking hook failure must not stop the tool; output:\n%s", out)
		})
	}
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_04_PreToolUseExit2BlocksToolAndShowsStderr(t *testing.T) {
	dir := t.TempDir()
	toolFile := filepath.Join(dir, "tool-ran")
	sess := filepath.Join(dir, "sess")
	writeSettings(t, dir, map[string]string{
		"PreToolUse": writeHook(t, dir, "pre.sh", exitHookScript("", "Blocked: Bash is off", 2)),
	})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-p2", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.False(t, fileExists(toolFile), "the blocked tool must not run")
	got := readOrEmpty(sess)
	assert.Contains(t, got, "PreToolUse:Bash hook error", "recorded run: tool_result is 'PreToolUse:Bash hook error: [cmd]: stderr'")
	assert.Contains(t, got, "Blocked: Bash is off", "the stderr reaches the model in the tool_result")
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_05_PostToolUseExit2ShowsStderrAfterToolRan(t *testing.T) {
	dir := t.TempDir()
	toolFile := filepath.Join(dir, "tool-ran")
	sess := filepath.Join(dir, "sess")
	writeSettings(t, dir, map[string]string{
		"PostToolUse": writeHook(t, dir, "post.sh", exitHookScript("", "post-tool warning", 2)),
	})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-p3", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "PostToolUse runs after the tool")
	assert.Contains(t, readOrEmpty(sess), "post-tool warning", "docs: exit 2 shows stderr to Claude")
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_06_SessionEndExit1AndSessionStartExit2AreNonBlocking(t *testing.T) {
	for _, tc := range []struct {
		event string
		code  int
	}{{"SessionEnd", 1}, {"SessionStart", 2}} {
		t.Run(tc.event, func(t *testing.T) {
			dir := t.TempDir()
			ran := filepath.Join(dir, "scenario-ran")
			writeSettings(t, dir, map[string]string{
				tc.event: writeHook(t, dir, "h.sh", exitHookScript("", "hook stderr", tc.code)),
			})
			script := writeScript(t, dir, "s.sh", "#!/bin/sh\necho x > \""+ran+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-p4", "--project-dir", dir, "-p", "go")
			assert.Equal(t, 0, code, "mock must exit 0; output:\n%s", out)
			assert.True(t, fileExists(ran), "the run completes")
		})
	}
}

// sr:docs https://code.claude.com/docs/en/hooks#stop
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_07_StopExit2ThenZeroRunsModelAgain(t *testing.T) {
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	sess := filepath.Join(dir, "sess")
	once := filepath.Join(dir, "stopped-once")
	writeSettings(t, dir, map[string]string{
		"Stop": writeHook(t, dir, "stop.sh", `cat >/dev/null
if [ ! -f "`+once+`" ]; then : > "`+once+`"; echo "reply STOPPED-ONCE" >&2; exit 2; fi
exit 0`),
	})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\necho run >> \""+runs+"\"\ncp \"$A10N_MOCK_SESSION_FILE\" \""+sess+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-p5", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Equal(t, 2, strings.Count(readOrEmpty(runs), "run"), "the scenario runs a second time after the Stop block; output:\n%s", out)
	got := readOrEmpty(sess)
	assert.Contains(t, got, "Stop hook feedback", "recorded run: the feedback is a meta user message 'Stop hook feedback:\\n[cmd]: stderr'")
	assert.Contains(t, got, "reply STOPPED-ONCE")
}

// sr:docs https://code.claude.com/docs/en/hooks#precompact
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_08_PreCompactExit2BlocksCompaction(t *testing.T) {
	for _, tc := range []struct {
		name      string
		code      int
		compacted bool
	}{{"exit 0 compacts", 0, true}, {"exit 2 blocks", 2, false}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			postLog := filepath.Join(dir, "post.log")
			sess := filepath.Join(dir, "sess")
			writeSettings(t, dir, map[string]string{
				"PreCompact":  writeHook(t, dir, "pre.sh", exitHookScript("", "no compaction", tc.code)),
				"PostCompact": writeHook(t, dir, "post.sh", "cat >/dev/null\necho fired >> \""+postLog+"\""),
			})
			script := writeScript(t, dir, "s.sh", "#!/bin/sh\n"+
				"printf '%s\\n' '{\"type\":\"compact\",\"summary\":\"the summary\"}'\n"+
				"cp \"$A10N_MOCK_SESSION_FILE\" \""+sess+"\" 2>/dev/null\n"+
				"printf '%s\\n' '"+resultFrame+"'\n")
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-cp", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, "output:\n%s", out)
			var all strings.Builder
			_ = filepath.Walk(cfg, func(p string, i os.FileInfo, err error) error {
				if err == nil && !i.IsDir() && strings.HasSuffix(p, ".jsonl") {
					all.WriteString(readOrEmpty(p))
				}
				return nil
			})
			assert.Equal(t, tc.compacted, strings.Contains(all.String(), "compact_boundary"), "compact_boundary in the transcript")
			assert.Equal(t, tc.compacted, fileExists(postLog), "PostCompact fires only when the compaction happens")
		})
	}
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_09_PostToolUseFailureExit2ShowsStderr(t *testing.T) {
	dir := t.TempDir()
	toolFile := filepath.Join(dir, "tool-ran")
	failLog := filepath.Join(dir, "fail.log")
	sess := filepath.Join(dir, "sess")
	writeSettings(t, dir, map[string]string{
		"PostToolUseFailure": writeHook(t, dir, "pf.sh", "cat >/dev/null\necho fired >> \""+failLog+"\"\necho 'failure-stderr-for-claude' >&2\nexit 2"),
	})
	script := toolScenarioCmd(t, dir, filepath.Join(dir, "runs"), sess, "echo ran > "+toolFile+"; exit 1")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-pf", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "the tool ran")
	assert.True(t, fileExists(failLog), "a failing Bash fires PostToolUseFailure")
	assert.Contains(t, readOrEmpty(sess), "failure-stderr-for-claude", "docs: exit 2 shows stderr to Claude")
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_10_Exit2BeatsJSONAllow(t *testing.T) {
	dir := t.TempDir()
	toolFile := filepath.Join(dir, "tool-ran")
	sess := filepath.Join(dir, "sess")
	writeSettings(t, dir, map[string]string{
		"PreToolUse": writeHook(t, dir, "pre.sh", exitHookScript(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`, "exit-2-wins", 2)),
	})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-aw", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.False(t, fileExists(toolFile), "exit 2 blocks even with permissionDecision allow")
	assert.Contains(t, readOrEmpty(sess), "exit-2-wins")
}
