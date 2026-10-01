package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_14_StopFeedbackInTheMainFile: a Stop block is recorded in the
// session's own file the way claude 2.1.282 records it. A JSON decision:block
// leaves the "Stop hook feedback:\n<reason>" meta turn, a hook_blocking_error
// {blockingError: {blockingError, command}}, then a stop_hook_summary (same
// toolUseID, hookErrors [reason], hasOutput true). An exit 2 leaves the
// feedback quoting "[<command>]: <stderr>" and the summary, no attachment.
// stop_hook_active is set on every Stop after a block.
// sr:proves hook-exit-code-semantics/claude
// staged:proves hook-output-transcript-records/claude
// staged:proves stop-block-continuation/claude
// sr:proves stop-hook-payload/claude
func TestT017_14_StopFeedbackInTheMainFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	counter := filepath.Join(dir, "n")
	hook := payloadLogger(t, dir, "stop.sh", log, `N=$(cat `+counter+` 2>/dev/null || echo 0); N=$((N+1)); echo $N > `+counter+`
if [ $N = 1 ]; then echo '{"decision":"block","reason":"JSON-REASON"}'; exit 0; fi
if [ $N = 2 ]; then echo "EXIT2-REASON" 1>&2; exit 2; fi`)
	settings(t, dir, map[string]string{"Stop": hook})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"turn text"}]}}'
echo '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "stop-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "stop-1"))
	fb := indexWhere(recs, 0, func(r rec) bool { return strings.HasPrefix(messageText(r), "Stop hook feedback:") })
	require.GreaterOrEqual(t, fb, 0)
	assert.Equal(t, "Stop hook feedback:\nJSON-REASON", messageText(recs[fb]))
	assert.Contains(t, recs[fb].Raw, `"isMeta":true`)
	att := recs[fb+1]
	assert.Equal(t, "hook_blocking_error", att.Attachment["type"])
	assert.Equal(t, map[string]any{"blockingError": "JSON-REASON", "command": hook}, att.Attachment["blockingError"])
	sum := recs[fb+2]
	require.True(t, isStopSummary(sum))
	var s1 map[string]any
	require.NoError(t, json.Unmarshal([]byte(sum.Raw), &s1))
	assert.Equal(t, att.Attachment["toolUseID"], s1["toolUseID"])
	assert.Equal(t, []any{"JSON-REASON"}, s1["hookErrors"])
	assert.Equal(t, true, s1["hasOutput"])
	assert.EqualValues(t, 1, s1["hookCount"])
	assert.Equal(t, []any{map[string]any{"command": hook}}, s1["hookInfos"], "a blocking hook is listed without durationMs")
	assert.Equal(t, "suggestion", s1["level"])

	fb2 := indexWhere(recs, fb+3, func(r rec) bool { return strings.HasPrefix(messageText(r), "Stop hook feedback:") })
	require.Greater(t, fb2, fb)
	assert.Equal(t, "Stop hook feedback:\n["+hook+"]: EXIT2-REASON\n", messageText(recs[fb2]))
	require.True(t, isStopSummary(recs[fb2+1]), "an exit-2 block leaves no attachment")
	last := indexWhere(recs, fb2+2, isStopSummary)
	require.Greater(t, last, fb2)
	var s3 map[string]any
	require.NoError(t, json.Unmarshal([]byte(recs[last].Raw), &s3))
	assert.Equal(t, false, s3["hasOutput"])
	assert.Equal(t, []any{}, s3["hookErrors"])

	var active []any
	for _, p := range payloads(t, log) {
		active = append(active, p["stop_hook_active"])
	}
	assert.Equal(t, []any{false, true, true}, active)
}

// TestT017_15_AdditionalContext: a hook whose JSON carries additionalContext
// leaves a hook_success (content "") and then a hook_additional_context — for
// PostToolUse under the tool call's name and id, for SessionStart named
// "SessionStart" with "SessionStart" as its toolUseID (claude 2.1.282).
// sr:proves hook-additional-context/claude
func TestT017_15_AdditionalContext(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	post := write(t, filepath.Join(dir, "post.sh"), `#!/bin/sh
cat >/dev/null
echo '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"POST-CTX"}}'
`, 0o755)
	ss := write(t, filepath.Join(dir, "ss.sh"), `#!/bin/sh
cat >/dev/null
echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"SS-CTX"}}'
`, 0o755)
	settings(t, dir, map[string]string{"PostToolUse": post, "SessionStart": ss})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "ac-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "ac-1"))
	var kinds []string
	for i, r := range recs {
		if r.Type != "attachment" {
			continue
		}
		kinds = append(kinds, r.Attachment["type"].(string)+":"+r.Attachment["hookName"].(string))
		if r.Attachment["type"] == "hook_additional_context" {
			prev := recs[i-1]
			assert.Equal(t, "hook_success", prev.Attachment["type"], "it follows the hook's hook_success")
			assert.Equal(t, "", prev.Attachment["content"], "whose content is empty for a JSON stdout")
		}
	}
	assert.Equal(t, []string{
		"hook_success:SessionStart:startup", "hook_additional_context:SessionStart",
		"hook_success:PostToolUse:Bash", "hook_additional_context:PostToolUse:Bash",
	}, kinds)
	for _, r := range recs {
		if r.Attachment["type"] != "hook_additional_context" {
			continue
		}
		if r.Attachment["hookEvent"] == "SessionStart" {
			assert.Equal(t, "SessionStart", r.Attachment["toolUseID"])
			assert.Equal(t, []any{"SS-CTX"}, r.Attachment["content"])
		} else {
			assert.True(t, strings.HasPrefix(r.Attachment["toolUseID"].(string), "b1"))
			assert.Equal(t, []any{"POST-CTX"}, r.Attachment["content"])
		}
	}
}

// TestT017_19_PromptAndSessionEndAttachments: a UserPromptSubmit hook that
// prints plain text leaves a hook_success whose content is that text; a
// SessionEnd hook's output leaves nothing (claude 2.1.282).
// staged:proves hook-output-transcript-records/claude
// sr:proves session-end-hook/claude
func TestT017_19_PromptAndSessionEndAttachments(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	ups := write(t, filepath.Join(dir, "ups.sh"), "#!/bin/sh\ncat >/dev/null\necho UPS-PLAIN-OUT\n", 0o755)
	se := write(t, filepath.Join(dir, "se.sh"), "#!/bin/sh\ncat >/dev/null\necho SE-OUT\necho SE-ERR 1>&2\n", 0o755)
	settings(t, dir, map[string]string{"UserPromptSubmit": ups, "SessionEnd": se})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "up-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "up-1"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Type == "attachment" })
	require.GreaterOrEqual(t, at, 1)
	assert.Equal(t, "e2e-root-up-1", recs[at-1].UUID, "it follows the prompt")
	assert.Equal(t, "hook_success", recs[at].Attachment["type"])
	assert.Equal(t, "UserPromptSubmit", recs[at].Attachment["hookName"])
	assert.Equal(t, "UPS-PLAIN-OUT", recs[at].Attachment["content"])
	raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "up-1"))
	assert.NotContains(t, string(raw), "SE-OUT")
	assert.NotContains(t, string(raw), "SE-ERR")
}

// TestT017_20_PostToolUsePayload: PostToolUse sends tool_response — the
// tool's structured result, a Bash's {stdout, stderr, interrupted, isImage,
// noOutputExpected} — never tool_output (docs, PostToolUse input; a claude
// 2.1.282 payload).
// staged:proves bash-tool-result/claude
// sr:proves posttooluse-payload/claude
func TestT017_20_PostToolUsePayload(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo POST-OUT"}`)), "--session-id", "pt-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.NotContains(t, ps[0], "tool_output")
	assert.Equal(t, map[string]any{"stdout": "POST-OUT", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false}, ps[0]["tool_response"])
}

// TestT017_21_InlineToolResultPostToolUseIsKeyedByTheCall: a tool_result the
// scenario wrote itself fires PostToolUse with the call's tool_use_id, and the
// attachment is keyed by it — as every one of the 1,651 real PostToolUse
// attachments is.
func TestT017_21_InlineToolResultPostToolUseIsKeyedByTheCall(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	post := write(t, filepath.Join(dir, "post.sh"), "#!/bin/sh\ncat >/dev/null\necho post-said 1>&2\n", 0o755)
	settings(t, dir, map[string]string{"PostToolUse": post})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_inline1","name":"AskUserQuestion","content":"yes"}]}}'
echo '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "il-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "il-1"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Type == "attachment" })
	require.GreaterOrEqual(t, at, 0)
	assert.Equal(t, "PostToolUse:AskUserQuestion", recs[at].Attachment["hookName"])
	assert.Equal(t, "toolu_inline1", recs[at].Attachment["toolUseID"])
}

// TestT017_26_StopBlockCap: a Stop hook that always blocks continues the turn
// CLAUDE_CODE_STOP_HOOK_BLOCK_CAP times (default 8); the next block is
// overridden, a warning is recorded, and the turn ends — claude 2.1.282 fired
// Stop 9 times and wrote exactly this warning. The stream carries one result,
// at the real end of the continued turn.
// staged:proves stop-block-cap/claude
// staged:proves stop-block-continuation/claude
func TestT017_26_StopBlockCap(t *testing.T) {
	for _, tc := range []struct {
		cap   string
		fires int
	}{{"", 9}, {"2", 3}} {
		t.Run("cap="+tc.cap, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "payloads.log")
			settings(t, dir, map[string]string{"Stop": payloadLogger(t, dir, "stop.sh", log, `echo '{"decision":"block","reason":"KEEP GOING"}'`)})
			sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"DONE"}]}}'
echo '{"type":"result","subtype":"success","result":"DONE"}'
`, 0o755)
			var env []string
			if tc.cap != "" {
				env = []string{"CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=" + tc.cap}
			}
			out, code := runInDir(t, dir, env, "--script", sc, "--session-id", "cap-1",
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)
			ps := payloads(t, log)
			require.Len(t, ps, tc.fires)
			for i, p := range ps {
				assert.Equal(t, i > 0, p["stop_hook_active"], "fire %d", i)
			}
			assert.Equal(t, 1, strings.Count(out, `"type":"result"`), "one result for the whole continued turn")
			assert.Contains(t, out, `"result":""`, "after the override the result carries no text, as claude 2.1.282 streamed it")
			recs := readRecs(t, transcriptPath(t, cfg, dir, "cap-1"))
			last := recs[len(recs)-1]
			assert.Equal(t, "informational", last.Subtype)
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(last.Raw), &m))
			assert.Equal(t, "warning", m["level"])
			assert.Equal(t, "A hook blocked the turn from ending "+strconv.Itoa(tc.fires)+" consecutive times — overriding and ending turn. For Stop/SubagentStop hooks, check stop_hook_active in the input and return success while it's true. Set CLAUDE_CODE_STOP_HOOK_BLOCK_CAP to raise this limit.", m["content"])
		})
	}
}

// TestT017_27_SubagentStopBlockCap: SubagentStop shares the cap — 9 fires by
// default, the sub-agent re-run 8 times — and, as in the controlled 2.1.282
// run, leaves no warning record in either file.
// staged:proves stop-block-cap/claude
func TestT017_27_SubagentStopBlockCap(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SubagentStop": payloadLogger(t, dir, "stop.sh", log, `echo '{"decision":"block","reason":"KEEP GOING"}'`)})
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"HELPED"}]}}'
echo '{"type":"result","subtype":"success","result":"HELPED"}'
`, 0o755)
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"fg","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "scap-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Len(t, payloads(t, log), 9)
	raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "scap-1"))
	assert.NotContains(t, string(raw), "informational")
}
