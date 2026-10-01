package e2e

import (
	"encoding/json"
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
	cfg := filepath.Join(dir, "config")
	marker := filepath.Join(dir, "scenario-ran")
	hook := writeHook(t, dir, "ups.sh", exitHookScript("", "prompt refused", 2))
	writeSettings(t, dir, map[string]string{"UserPromptSubmit": hook})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\necho x > \""+marker+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-b", "--project-dir", dir, "--config-dir", cfg, "-p", "hi")
	t.Logf("mock exit=%d output:\n%s", code, out)
	assert.False(t, fileExists(marker), "a blocked prompt never reaches the model: the scenario must not run")
	// Recording snapshots/runs/prompt-blocked: real claude exits 0.
	assert.Equal(t, 0, code)
	want := "UserPromptSubmit operation blocked by hook:\n[" + hook + "]: prompt refused\n\n\nOriginal prompt: hi"

	var info, result map[string]any
	for _, l := range strings.Split(out, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil {
			continue
		}
		switch {
		case m["type"] == "system" && m["subtype"] == "informational":
			info = m
		case m["type"] == "result":
			result = m
		}
	}
	require.NotNil(t, info, "a stream system informational frame")
	assert.Equal(t, want, info["content"])
	assert.Equal(t, "warning", info["level"])
	assert.Equal(t, true, info["prevent_continuation"])
	require.NotNil(t, result, "a result frame")
	assert.Equal(t, "success", result["subtype"])
	assert.Equal(t, false, result["is_error"])
	assert.EqualValues(t, 0, result["num_turns"])
	assert.Equal(t, want, result["result"])

	var rec map[string]any
	for _, r := range allRecords(t, cfg) {
		if r["type"] == "system" && r["subtype"] == "informational" {
			rec = r
		}
	}
	require.NotNil(t, rec, "a transcript system informational record")
	assert.Equal(t, want, rec["content"])
	assert.Equal(t, "warning", rec["level"])
	assert.Equal(t, true, rec["preventContinuation"])
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
			cfg := filepath.Join(dir, "config")
			ran := filepath.Join(dir, "scenario-ran")
			hook := writeHook(t, dir, "h.sh", exitHookScript("", "hook stderr", tc.code))
			writeSettings(t, dir, map[string]string{tc.event: hook})
			script := writeScript(t, dir, "s.sh", "#!/bin/sh\necho x > \""+ran+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-p4", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			assert.Equal(t, 0, code, "mock must exit 0; output:\n%s", out)
			assert.True(t, fileExists(ran), "the run completes")
			recs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
			if tc.event == "SessionEnd" {
				// The recording's transcript holds no record of SessionEnd's exit 1.
				assert.Empty(t, recs)
				assert.NotContains(t, allTranscriptText(cfg), "hook stderr")
				return
			}
			// Recorded shape (claude 2.1.285, SessionStart exit 2).
			require.Len(t, recs, 1)
			assert.Equal(t, map[string]any{
				"type": "hook_non_blocking_error", "hookName": "SessionStart:startup", "hookEvent": "SessionStart",
				"stderr": "[" + hook + "]: hook stderr\n", "stdout": "", "exitCode": float64(2), "command": hook,
			}, withoutKeys(recs[0], "toolUseID", "durationMs"))
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
	// Recorded (claude 2.1.285): a hook_blocking_error attachment, named
	// PostToolUseFailure:<Tool>, blockingError.blockingError "[cmd]: stderr\n".
	hook := filepath.Join(dir, "pf.sh")
	blk := attachmentsOf(readRecordsFile(t, sess), "hook_blocking_error")
	require.Len(t, blk, 1)
	assert.Equal(t, "PostToolUseFailure:Bash", blk[0]["hookName"])
	assert.Equal(t, "PostToolUseFailure", blk[0]["hookEvent"])
	assert.Equal(t, map[string]any{"blockingError": "[" + hook + "]: failure-stderr-for-claude\n", "command": hook}, blk[0]["blockingError"])
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

// ---- helpers over transcript records ----

func readRecordsFile(t *testing.T, p string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(readOrEmpty(p), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), "line: %s", l)
		out = append(out, m)
	}
	return out
}

func transcriptFiles(cfg string) []string {
	var fs []string
	_ = filepath.Walk(cfg, func(p string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() && strings.HasSuffix(p, ".jsonl") {
			fs = append(fs, p)
		}
		return nil
	})
	return fs
}

func allRecords(t *testing.T, cfg string) []map[string]any {
	var out []map[string]any
	for _, f := range transcriptFiles(cfg) {
		out = append(out, readRecordsFile(t, f)...)
	}
	return out
}

func allTranscriptText(cfg string) string {
	var b strings.Builder
	for _, f := range transcriptFiles(cfg) {
		b.WriteString(readOrEmpty(f))
	}
	return b.String()
}

func attachmentsOf(recs []map[string]any, typ string) []map[string]any {
	var out []map[string]any
	for _, r := range recs {
		if a, ok := r["attachment"].(map[string]any); ok && a["type"] == typ {
			out = append(out, a)
		}
	}
	return out
}

// modelText is the text of every user-role message record: what the model is told.
func modelText(recs []map[string]any) string {
	var b strings.Builder
	for _, r := range recs {
		if r["type"] == "user" {
			j, _ := json.Marshal(r["message"])
			b.Write(j)
		}
	}
	return b.String()
}

func withoutKeys(m map[string]any, keys ...string) map[string]any {
	c := map[string]any{}
	for k, v := range m {
		c[k] = v
	}
	for _, k := range keys {
		delete(c, k)
	}
	return c
}

// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_12_PostToolUseExit1IsNonBlockingNotice(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "sess")
	hook := writeHook(t, dir, "post.sh", exitHookScript("", "post-tool warning", 1))
	writeSettings(t, dir, map[string]string{"PostToolUse": hook})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, filepath.Join(dir, "tool-ran"))
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-12", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	recs := readRecordsFile(t, sess)
	nb := attachmentsOf(recs, "hook_non_blocking_error")
	require.Len(t, nb, 1, "one hook_non_blocking_error attachment")
	// Recorded (claude 2.1.285): stderr is the first line prefixed "Failed with non-blocking status code: ".
	assert.Equal(t, map[string]any{
		"type": "hook_non_blocking_error", "hookName": "PostToolUse:Bash", "hookEvent": "PostToolUse",
		"stderr": "Failed with non-blocking status code: post-tool warning", "stdout": "", "exitCode": float64(1), "command": hook,
	}, withoutKeys(nb[0], "toolUseID", "durationMs"))
	assert.Empty(t, attachmentsOf(recs, "hook_blocking_error"), "not a blocking error")
	assert.NotContains(t, modelText(recs), "post-tool warning", "the stderr is not a message to the model")
}

// sr:docs https://code.claude.com/docs/en/hooks#stop-input
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_13_StopHookActiveFalseThenTrue(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "stop.log")
	once := filepath.Join(dir, "once")
	writeSettings(t, dir, map[string]string{
		"Stop": writeHook(t, dir, "stop.sh", `grep -o '"stop_hook_active":[a-z]*' >> "`+log+`"
if [ ! -f "`+once+`" ]; then : > "`+once+`"; echo again >&2; exit 2; fi
exit 0`),
	})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-13", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Equal(t, "\"stop_hook_active\":false\n\"stop_hook_active\":true\n", readOrEmpty(log))
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_14a_SubagentStopExit2ReRunsSubagent(t *testing.T) {
	dir := t.TempDir()
	runs := filepath.Join(dir, "sub-runs")
	once := filepath.Join(dir, "once")
	writeSettings(t, dir, map[string]string{
		"SubagentStop": writeHook(t, dir, "ss.sh", `cat >/dev/null
if [ ! -f "`+once+`" ]; then : > "`+once+`"; echo "sub not done" >&2; exit 2; fi
exit 0`),
	})
	sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\necho run >> \""+runs+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", sub))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "s-14a", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Equal(t, 2, strings.Count(readOrEmpty(runs), "run"), "the subagent runs again after SubagentStop exit 2")
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_14b_SubagentStartExit2IsNonBlockingAndHiddenFromModel(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	ran := filepath.Join(dir, "sub-ran")
	writeSettings(t, dir, map[string]string{
		"SubagentStart": writeHook(t, dir, "sa.sh", exitHookScript("", "substart-secret-stderr", 2)),
	})
	sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\necho x > \""+ran+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
	orch := writeScript(t, dir, "orch.sh", orchestratorScript("Agent", sub))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "s-14b", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(ran), "the subagent still runs")
	assert.NotContains(t, modelText(allRecords(t, cfg)), "substart-secret-stderr", "docs: stderr is not shown to Claude")
}

// StopFailure cannot be triggered from a scenario: the mock does not model it
// (internal/runner/runner.go: "StopFailure, which the mock does not model").
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_14c_PostCompactExit2ShowsStderrToUserNotModel(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	writeSettings(t, dir, map[string]string{
		"PostCompact": writeHook(t, dir, "pc.sh", exitHookScript("", "postcompact-secret-stderr", 2)),
	})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"compact\",\"summary\":\"sum\"}'\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-14c", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Contains(t, allTranscriptText(cfg), "compact_boundary", "the compaction happened")
	assert.NotContains(t, modelText(allRecords(t, cfg)), "postcompact-secret-stderr", "docs: stderr goes to the user only")
}

// Recording snapshots/runs/hook-exit-json: an exit-0 hook's stderr IS carried by
// its hook_success attachment (the docs are wrong about the transcript), but is
// never a message the model is told.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_15_Exit0StderrIsOnlyInHookSuccessAttachment(t *testing.T) {
	for _, event := range []string{"PreToolUse", "PostToolUse", "UserPromptSubmit", "Stop"} {
		t.Run(event, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			sess := filepath.Join(dir, "sess")
			toolFile := filepath.Join(dir, "tool-ran")
			hook := writeHook(t, dir, "h.sh", exitHookScript("", "quiet-debug-stderr", 0))
			writeSettings(t, dir, map[string]string{event: hook})
			script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-15", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, "output:\n%s", out)
			assert.True(t, fileExists(toolFile))
			recs := allRecords(t, cfg)
			assert.NotContains(t, modelText(recs), "quiet-debug-stderr", "the model is not told the stderr")
			assert.Empty(t, attachmentsOf(recs, "hook_blocking_error"))
			assert.Empty(t, attachmentsOf(recs, "hook_non_blocking_error"))
			if event == "PreToolUse" {
				ok := attachmentsOf(recs, "hook_success")
				require.Len(t, ok, 1)
				assert.Equal(t, map[string]any{
					"type": "hook_success", "hookName": "PreToolUse:Bash", "hookEvent": "PreToolUse", "content": "",
					"stdout": "", "stderr": "quiet-debug-stderr\n", "exitCode": float64(0), "command": hook,
				}, withoutKeys(ok[0], "toolUseID", "durationMs"))
			}
		})
	}
}

// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_16_Exit1WithJSONDenyLetsJSONDecide(t *testing.T) {
	dir := t.TempDir()
	toolFile := filepath.Join(dir, "tool-ran")
	sess := filepath.Join(dir, "sess")
	writeSettings(t, dir, map[string]string{
		"PreToolUse": writeHook(t, dir, "pre.sh", exitHookScript(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"json-says-no"}}`, "", 1)),
	})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-16", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.False(t, fileExists(toolFile), "the JSON deny decides, whatever the exit code")
	assert.Contains(t, readOrEmpty(sess), "json-says-no")
	assert.Empty(t, attachmentsOf(readRecordsFile(t, sess), "hook_non_blocking_error"), "the hook is not reported as an error")
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_17_Exit2WithInvalidJSONStillBlocksWithStderr(t *testing.T) {
	dir := t.TempDir()
	toolFile := filepath.Join(dir, "tool-ran")
	sess := filepath.Join(dir, "sess")
	writeSettings(t, dir, map[string]string{
		"PreToolUse": writeHook(t, dir, "pre.sh", exitHookScript(`{"decision": 42}`, "stderr-is-the-reason", 2)),
	})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-17", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.False(t, fileExists(toolFile), "exit 2 blocks even when the JSON fails validation")
	assert.Contains(t, readOrEmpty(sess), "stderr-is-the-reason")
}

// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_18_SessionStartExit2StderrNotSeenByModel(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sess := filepath.Join(dir, "sess")
	ctxLog := filepath.Join(dir, "ctx")
	hook := writeHook(t, dir, "ss.sh", exitHookScript("", "session-start-secret", 2))
	writeSettings(t, dir, map[string]string{"SessionStart": hook})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, filepath.Join(dir, "tool-ran"))
	script = writeScript(t, dir, "wrap.sh", "#!/bin/sh\nprintf %s \"$A10N_MOCK_ADDITIONAL_CONTEXT\" >> \""+ctxLog+"\"\nexec "+script+"\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-18", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.NotContains(t, readOrEmpty(ctxLog), "session-start-secret", "not in the context the model gets")
	recs := allRecords(t, cfg)
	assert.NotContains(t, modelText(recs), "session-start-secret", "not in any user message or tool_result")
	nb := attachmentsOf(recs, "hook_non_blocking_error")
	require.Len(t, nb, 1, "only the notice carries it")
	assert.Equal(t, "["+hook+"]: session-start-secret\n", nb[0]["stderr"])
}

// hookWithRaw is a hook printing stdout with NO trailing newline (as the recorded
// hook does), writing stderr, and exiting code.
func hookWithRaw(t *testing.T, dir, stdout, stderr string, code int) string {
	body := "cat >/dev/null\nprintf '%s' '" + stdout + "'\n"
	if stderr != "" {
		body += "printf '%s\\n' '" + stderr + "' >&2\n"
	}
	return writeHook(t, dir, "raw.sh", body+"exit "+string(rune('0'+code))+"\n")
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_19_Exit0MalformedJSONIsNonBlockingError(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "sess")
	toolFile := filepath.Join(dir, "tool-ran")
	hook := hookWithRaw(t, dir, "{not json at all}", "", 0)
	writeSettings(t, dir, map[string]string{"PreToolUse": hook})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-19", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "the tool still runs")
	nb := attachmentsOf(readRecordsFile(t, sess), "hook_non_blocking_error")
	require.Len(t, nb, 1)
	assert.Equal(t, "PreToolUse:Bash", nb[0]["hookName"])
	assert.True(t, strings.HasPrefix(nb[0]["stderr"].(string), "Hook output looks like a JSON object but is not valid JSON"), "stderr: %v", nb[0]["stderr"])
	assert.Equal(t, "{not json at all}", nb[0]["stdout"])
	assert.EqualValues(t, 0, nb[0]["exitCode"])
}

// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_20_Exit1SchemaInvalidJSONIsNonBlockingError(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "sess")
	toolFile := filepath.Join(dir, "tool-ran")
	hook := hookWithRaw(t, dir, `{"decision": 42}`, "schema-invalid on exit 1", 1)
	writeSettings(t, dir, map[string]string{"PreToolUse": hook})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-20", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "the tool still runs")
	nb := attachmentsOf(readRecordsFile(t, sess), "hook_non_blocking_error")
	require.Len(t, nb, 1)
	stderr := nb[0]["stderr"].(string)
	assert.True(t, strings.HasPrefix(stderr, "Hook JSON output validation failed"), "stderr: %s", stderr)
	assert.Contains(t, stderr, "Hook exited 1 with stderr:\nschema-invalid on exit 1")
	assert.EqualValues(t, 1, nb[0]["exitCode"])
}

// sr:docs https://code.claude.com/docs/en/hooks#other-exit-codes
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_21_UserPromptSubmitAndStopExit1AreNonBlocking(t *testing.T) {
	for _, event := range []string{"UserPromptSubmit", "Stop"} {
		t.Run(event, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			runs := filepath.Join(dir, "runs")
			hook := writeHook(t, dir, "h.sh", exitHookScript("", "hook failed with exit 1", 1))
			writeSettings(t, dir, map[string]string{event: hook})
			script := writeScript(t, dir, "s.sh", "#!/bin/sh\necho run >> \""+runs+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-21", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, "output:\n%s", out)
			assert.Equal(t, 1, strings.Count(readOrEmpty(runs), "run"), "the prompt proceeds and the turn ends: no re-run")
			nb := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
			require.Len(t, nb, 1)
			assert.Equal(t, map[string]any{
				"type": "hook_non_blocking_error", "hookName": event, "hookEvent": event,
				"stderr": "Failed with non-blocking status code: hook failed with exit 1", "stdout": "", "exitCode": float64(1), "command": hook,
			}, withoutKeys(nb[0], "toolUseID", "durationMs"))
		})
	}
}
