package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	// Docs: "the notice appears in the subagent's own transcript, not in the parent conversation."
	inSub, inParent := 0, 0
	for _, f := range transcriptFiles(cfg) {
		n := 0
		for _, a := range attachmentsOf(readRecordsFile(t, f), "hook_non_blocking_error") {
			if strings.Contains(a["stderr"].(string), "substart-secret-stderr") {
				n++
			}
		}
		if filepath.Base(f) == "s-14b.jsonl" {
			inParent += n
		} else {
			inSub += n
			t.Logf("notice found in %s", f)
		}
	}
	assert.Equal(t, 1, inSub, "the notice is in the subagent's own transcript")
	assert.Equal(t, 0, inParent, "and not in the parent session's transcript")
}

// PostCompact exit 2 shows its stderr to the user only: a manual compaction
// reports "PostCompact [<command>] failed: <stderr>" in the /compact command
// output, and nowhere else.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT009_10_14c_PostCompactExit2ShowsStderrToUserNotModel(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	hook := writeHook(t, dir, "pc.sh", exitHookScript("", "postcompact-secret-stderr", 2))
	writeSettings(t, dir, map[string]string{"PostCompact": hook})
	script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"compact\",\"summary\":\"sum\",\"trigger\":\"manual\"}'\nprintf '%s\\n' '"+resultFrame+"'\n")
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-14c", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Contains(t, allTranscriptText(cfg), "compact_boundary", "the compaction happened")
	assert.Contains(t, allTranscriptText(cfg), "PostCompact ["+hook+"] failed: postcompact-secret-stderr",
		"the user sees the failure and its stderr in /compact's output")
	for _, r := range allRecords(t, cfg) {
		if strings.Contains(fmt.Sprint(r), "postcompact-secret-stderr") {
			assert.Contains(t, fmt.Sprint(r), "<local-command-stdout>", "the stderr is only in the command output shown to the user")
		}
	}
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
// sr:proves session-start-hook/claude
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

// recordedDecisionInvalid is the message claude 2.1.285 gave a hook printing
// {"decision": 42} (recorded: snapshots/runs/hook-exit-json, cases d and g):
// it names the failing field and the values it allows.
const recordedDecisionInvalid = "Hook JSON output validation failed — decision: Invalid option: expected one of \"approve\"|\"block\""

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
	assert.True(t, strings.HasPrefix(stderr, recordedDecisionInvalid), "stderr: %s", stderr)
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

// plainTextStdoutCases are the recorded exit-0 outputs that are not a JSON
// output object, so they are plain text (hook_success, content = stdout).
var plainTextStdoutCases = []struct{ name, stdout string }{
	{"unclosed object", `{"unclosed": 1`},
	{"array and string lines", "[1, 2]\n\"just a string\""},
	{"object lines setting no field", "{\"x\": 1}\n{\"y\": 2}"},
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
// sr:proves hook-exit-code-semantics/claude
func TestT009_11_PlainTextLikeStdoutIsHookSuccessContent(t *testing.T) {
	for _, tc := range plainTextStdoutCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sess := filepath.Join(dir, "sess")
			toolFile := filepath.Join(dir, "tool-ran")
			hook := hookWithRaw(t, dir, tc.stdout, "", 0)
			writeSettings(t, dir, map[string]string{"PreToolUse": hook})
			script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-pt", "--project-dir", dir, "-p", "go")
			require.Equal(t, 0, code, "output:\n%s", out)
			assert.True(t, fileExists(toolFile), "the tool runs")
			recs := readRecordsFile(t, sess)
			assert.Empty(t, attachmentsOf(recs, "hook_non_blocking_error"), "no error")
			ok := attachmentsOf(recs, "hook_success")
			require.Len(t, ok, 1)
			assert.Equal(t, map[string]any{
				"type": "hook_success", "hookName": "PreToolUse:Bash", "hookEvent": "PreToolUse",
				"content": tc.stdout, "stdout": tc.stdout, "stderr": "", "exitCode": float64(0), "command": hook,
			}, withoutKeys(ok[0], "toolUseID", "durationMs"))
		})
	}
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
// sr:proves hook-exit-code-semantics/claude
func TestT009_11_Exit0SchemaInvalidJSONIsNonBlockingError(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "sess")
	toolFile := filepath.Join(dir, "tool-ran")
	hook := hookWithRaw(t, dir, `{"decision": 42}`, "", 0)
	writeSettings(t, dir, map[string]string{"PreToolUse": hook})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-g", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(toolFile), "the tool runs")
	nb := attachmentsOf(readRecordsFile(t, sess), "hook_non_blocking_error")
	require.Len(t, nb, 1)
	stderr := nb[0]["stderr"].(string)
	assert.True(t, strings.HasPrefix(stderr, recordedDecisionInvalid), "stderr: %s", stderr)
	assert.NotContains(t, stderr, "Hook exited", "exit 0 has no 'Hook exited' tail")
	assert.EqualValues(t, 0, nb[0]["exitCode"])
}

// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2
// sr:proves hook-exit-code-semantics/claude
func TestT009_11_Exit2JSONReasonBeatsStderr(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "sess")
	toolFile := filepath.Join(dir, "tool-ran")
	hook := hookWithRaw(t, dir, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"the JSON reason wins"}}`, "the stderr loses", 2)
	writeSettings(t, dir, map[string]string{"PreToolUse": hook})
	script := toolScenario(t, dir, filepath.Join(dir, "runs"), sess, toolFile)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-h", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.False(t, fileExists(toolFile), "the tool is blocked")
	var results []string
	for _, r := range readRecordsFile(t, sess) {
		if r["type"] != "user" {
			continue
		}
		msg, _ := r["message"].(map[string]any)
		parts, _ := msg["content"].([]any)
		for _, p := range parts {
			if m, ok := p.(map[string]any); ok && m["type"] == "tool_result" {
				results = append(results, m["content"].(string))
			}
		}
	}
	assert.Equal(t, []string{"PreToolUse:Bash hook error: the JSON reason wins"}, results, "no [cmd] prefix, no stderr")
}

// On the events whose plain-text stdout becomes context, stdout tried as JSON
// that does not parse is a non-blocking error, and its text is not added as
// context.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
// sr:proves hook-exit-code-semantics/claude
func TestT009_11_MalformedJSONOnContextEventsIsNotContext(t *testing.T) {
	for _, event := range []string{"UserPromptSubmit", "SessionStart"} {
		t.Run(event, func(t *testing.T) {
			dir := t.TempDir()
			ctxLog := filepath.Join(dir, "ctx.log")
			writeSettings(t, dir, map[string]string{event: hookWithRaw(t, dir, "{not json at all}", "", 0)})
			script := writeScript(t, dir, "s.sh", "#!/bin/sh\nprintf %s \"$A10N_MOCK_ADDITIONAL_CONTEXT\" > \""+ctxLog+"\"\nprintf '%s\\n' '"+resultFrame+"'\n")
			cfg := filepath.Join(dir, "config")
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-mj", "--project-dir", dir, "--config-dir", cfg, "-p", "hi")
			require.Equal(t, 0, code, "output:\n%s", out)
			assert.NotContains(t, readOrEmpty(ctxLog), "not json", "malformed JSON is not context")
			errs := attachmentsOf(allRecords(t, cfg), "hook_non_blocking_error")
			require.NotEmpty(t, errs, "malformed JSON is a non-blocking error")
			assert.True(t, strings.HasPrefix(errs[0]["stderr"].(string), "Hook output looks like a JSON object but is not valid JSON"))
		})
	}
}
