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

// TestT017_32_CommonFieldsOnEveryEvent: every lifecycle hook, on the main
// thread and inside a sub-agent, is told the session, its transcript file and
// its working directory (docs, common input fields; the hookmix and stops
// recordings). Inside the sub-agent, and only there, the payload also names
// the sub-agent: agent_id with agent_type, on its tool events and on
// SubagentStart and SubagentStop alike.
// sr:proves hook-common-payload/claude
func TestT017_32_CommonFieldsOnEveryEvent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{
		"SessionStart": h, "UserPromptSubmit": h, "PreToolUse": h, "PostToolUse": h,
		"SubagentStart": h, "SubagentStop": h, "Stop": h, "SessionEnd": h,
	})
	sub := script(t, dir, "sub", toolUse("sb1", "Bash", `{"command":"true"}`))
	orch := script(t, dir, "orch",
		toolUse("mb1", "Bash", `{"command":"true"}`),
		toolUse("ag1", "Agent", `{"prompt":"do it","description":"d","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "cf-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	wantPath := transcriptPath(t, cfg, dir, "cf-1")
	wantCwd, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	agentIDs := map[string]bool{}
	seen := map[string]bool{}
	var order []string
	for _, p := range payloads(t, log) {
		ev := p["hook_event_name"].(string)
		id, _ := p["tool_use_id"].(string)
		if strings.HasPrefix(id, "ag1") {
			order = append(order, ev+":Agent")
		} else {
			order = append(order, ev)
		}
		sub := ev == "SubagentStart" || ev == "SubagentStop" || strings.HasPrefix(id, "sb1")
		seen[ev] = true
		assert.Equal(t, "cf-1", p["session_id"], "%s names the session", ev)
		assert.Equal(t, wantPath, p["transcript_path"], "%s names the session's transcript", ev)
		gotCwd, err := filepath.EvalSymlinks(p["cwd"].(string))
		require.NoError(t, err)
		assert.Equal(t, wantCwd, gotCwd, "%s names the working directory", ev)
		if !sub {
			assert.NotContains(t, p, "agent_id", "%s on the main thread names no sub-agent", ev)
			assert.NotContains(t, p, "agent_type", "%s on the main thread names no sub-agent", ev)
			continue
		}
		assert.NotEmpty(t, p["agent_id"], "%s inside the sub-agent names it", ev)
		assert.Equal(t, "general-purpose", p["agent_type"], "%s inside the sub-agent names its type", ev)
		agentIDs[p["agent_id"].(string)] = true
	}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop", "SessionEnd"} {
		assert.True(t, seen[ev], "%s never fired", ev)
	}
	assert.Len(t, agentIDs, 1, "one sub-agent, one id, on every event it raised")
	// the Agent call is a main-thread call: it brackets the sub-agent's whole run
	pre, start, stop, post := -1, -1, -1, -1
	for i, ev := range order {
		switch ev {
		case "PreToolUse:Agent":
			pre = i
		case "SubagentStart":
			start = i
		case "SubagentStop":
			stop = i
		case "PostToolUse:Agent":
			post = i
		}
	}
	assert.True(t, pre >= 0 && pre < start && start < stop && stop < post, "order: %v", order)
}

// TestT017_33_PostToolUsePayloadFields: after a successful call a hook is told
// the tool's input exactly as given, the call's id, the structured response
// and how long the call took (duration_ms: the call's own time, so a command
// that sleeps shows it); a call a PreToolUse hook refused fires none (docs,
// PostToolUse input; the stops recording: 3 PreToolUse, 1 PostToolUse).
// sr:proves posttooluse-payload/claude
func TestT017_33_PostToolUsePayloadFields(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "post.log")
	post := payloadLogger(t, dir, "post.sh", log, "")
	pre := write(t, filepath.Join(dir, "pre.sh"), `#!/bin/sh
IN=$(cat)
case "$IN" in
  *REFUSEME*) echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"no"}}';;
  *EXIT2ME*) echo no >&2; exit 2;;
esac
exit 0
`, 0o755)
	settings(t, dir, map[string]string{"PostToolUse": post, "PreToolUse": pre})
	sc := script(t, dir, "s",
		toolUse("r1", "Bash", `{"command":"echo REFUSEME"}`),
		toolUse("r2", "Bash", `{"command":"echo EXIT2ME"}`),
		toolUse("b1", "Bash", `{"command":"sleep 0.3; echo SLEPT"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pp-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	ps := payloads(t, log)
	require.Len(t, ps, 1, "only the call that ran fires PostToolUse; the refused one fires none")
	p := ps[0]
	assert.Equal(t, "Bash", p["tool_name"])
	assert.Equal(t, map[string]any{"command": "sleep 0.3; echo SLEPT"}, p["tool_input"])
	assert.True(t, strings.HasPrefix(p["tool_use_id"].(string), "b1"), "the call's id")
	assert.Equal(t, "SLEPT", p["tool_response"].(map[string]any)["stdout"])
	d, ok := p["duration_ms"].(float64)
	require.True(t, ok, "duration_ms is a number: %v", p["duration_ms"])
	assert.GreaterOrEqual(t, d, 250.0, "the call's own run time")
	assert.Less(t, d, 5000.0)
}

// TestT017_34_PromptHookOrderAndNoRewrite: the prompt hook runs before the
// agent does, and what it prints cannot rewrite the prompt: a hook returning a
// replacement prompt in any field leaves the agent's prompt as submitted, while
// its additionalContext is added beside it; several hooks' contexts are all
// kept, in the order the hooks are configured (docs, UserPromptSubmit decision control, Add context for
// Claude).
// sr:proves user-prompt-submit-hook/claude
// sr:proves hook-additional-context/claude
func TestT017_34_PromptHookOrderAndNoRewrite(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	order := filepath.Join(dir, "order.log")
	hook := func(name, out string) string {
		return write(t, filepath.Join(dir, name), "#!/bin/sh\ncat >/dev/null\necho "+name+" >> "+order+"\necho '"+out+"'\n", 0o755)
	}
	h1 := hook("h1.sh", `{"prompt":"REWRITTEN","updatedPrompt":"REWRITTEN","userPrompt":"REWRITTEN","hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"CTX-ONE","updatedPrompt":"REWRITTEN"}}`)
	h2 := hook("h2.sh", `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"CTX-TWO"}}`)
	write(t, filepath.Join(dir, ".claude", "settings.json"), `{"hooks":{"UserPromptSubmit":[{"matcher":"*","hooks":[{"type":"command","command":"`+h1+`"},{"type":"command","command":"`+h2+`"}]}]}}`, 0o644)
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo agent >> `+order+`
printf %s "$A10N_MOCK_PROMPT" > `+filepath.Join(dir, "prompt.out")+`
printf %s "$A10N_MOCK_ADDITIONAL_CONTEXT" > `+filepath.Join(dir, "ctx.out")+`
echo '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "up-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "the submitted prompt")
	require.Equal(t, 0, code, out)

	got, err := os.ReadFile(order)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	require.Len(t, lines, 3)
	assert.ElementsMatch(t, []string{"h1.sh", "h2.sh"}, lines[:2], "both hooks run (together, in either order) before the agent")
	assert.Equal(t, "agent", lines[2])
	prompt, err := os.ReadFile(filepath.Join(dir, "prompt.out"))
	require.NoError(t, err)
	assert.Equal(t, "the submitted prompt", string(prompt), "no field of a hook's output rewrites the prompt")
	ctx, err := os.ReadFile(filepath.Join(dir, "ctx.out"))
	require.NoError(t, err)
	assert.Equal(t, "CTX-ONE\nCTX-TWO", string(ctx), "every hook's context is added")
}

// TestT017_36_EveryHooksContextIsRecorded: two hooks on one event each return
// JSON additionalContext and each leaves its own record, in hook order. After
// a tool call it is a hook_success then a hook_additional_context, named for
// the tool and keyed by the call's id, for an Agent call as for a Bash one; for
// a submitted prompt it is the hook_additional_context alone, named for the
// event (recorded: snapshots/runs/ctxmulti).
// sr:proves hook-additional-context/claude
// sr:proves user-prompt-submit-hook/claude
func TestT017_36_EveryHooksContextIsRecorded(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	hook := func(name, ev, ctx string) string {
		return write(t, filepath.Join(dir, name), "#!/bin/sh\ncat >/dev/null\necho '{\"hookSpecificOutput\":{\"hookEventName\":\""+ev+"\",\"additionalContext\":\""+ctx+"\"}}'\n", 0o755)
	}
	two := func(ev string) string {
		return `[{"matcher":"*","hooks":[{"type":"command","command":"` + hook(ev+"1.sh", ev, ev+"-ONE") + `"},{"type":"command","command":"` + hook(ev+"2.sh", ev, ev+"-TWO") + `"}]}]`
	}
	write(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"UserPromptSubmit":`+two("UserPromptSubmit")+`,"PostToolUse":`+two("PostToolUse")+`}}`, 0o644)
	sub := script(t, dir, "sub")
	orch := script(t, dir, "orch",
		toolUse("b1", "Bash", `{"command":"true"}`),
		toolUse("ag1", "Agent", `{"prompt":"p","description":"d","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "cx-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	var got []string
	for _, r := range readRecs(t, transcriptPath(t, cfg, dir, "cx-1")) {
		if r.Type != "attachment" || !strings.HasPrefix(r.Attachment["type"].(string), "hook_") {
			continue
		}
		line := r.Attachment["type"].(string) + " " + r.Attachment["hookName"].(string)
		if c, ok := r.Attachment["content"].([]any); ok {
			line += " " + c[0].(string)
		}
		if strings.HasPrefix(r.Attachment["hookName"].(string), "PostToolUse") {
			id := r.Attachment["toolUseID"].(string)
			line += " " + id[:strings.IndexAny(id, "@t")]
		}
		got = append(got, line)
	}
	assert.Equal(t, []string{
		"hook_additional_context UserPromptSubmit UserPromptSubmit-ONE",
		"hook_additional_context UserPromptSubmit UserPromptSubmit-TWO",
		"hook_success PostToolUse:Bash b1",
		"hook_additional_context PostToolUse:Bash PostToolUse-ONE b1",
		"hook_success PostToolUse:Bash b1",
		"hook_additional_context PostToolUse:Bash PostToolUse-TWO b1",
		"hook_success PostToolUse:Agent ag1",
		"hook_additional_context PostToolUse:Agent PostToolUse-ONE ag1",
		"hook_success PostToolUse:Agent ag1",
		"hook_additional_context PostToolUse:Agent PostToolUse-TWO ag1",
	}, got)
}

// TestT017_35_PromptRefusedByJSONDecision: a prompt hook that blocks by JSON
// (decision "block" with a reason) refuses the prompt as an exit 2 does: the
// agent never runs, the run ends successfully with the block message, which
// names the reason and, unless the hook asked to suppress it, the original
// prompt (docs, UserPromptSubmit decision control, What a blocked prompt leaves
// behind).
// sr:proves user-prompt-submit-hook/claude
func TestT017_35_PromptRefusedByJSONDecision(t *testing.T) {
	for _, tc := range []struct {
		name, hookOut string
		wantPrompt    bool
	}{
		{"original prompt shown", `{"decision":"block","reason":"NOT-ALLOWED"}`, true},
		{"original prompt suppressed", `{"decision":"block","reason":"NOT-ALLOWED","hookSpecificOutput":{"hookEventName":"UserPromptSubmit","suppressOriginalPrompt":true}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			ran := filepath.Join(dir, "ran")
			hook := write(t, filepath.Join(dir, "ups.sh"), "#!/bin/sh\ncat >/dev/null\necho '"+tc.hookOut+"'\n", 0o755)
			settings(t, dir, map[string]string{"UserPromptSubmit": hook})
			sc := write(t, filepath.Join(dir, "s.sh"), "#!/bin/sh\n: > "+ran+"\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"done\"}'\n", 0o755)
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pj-1",
				"--project-dir", dir, "--config-dir", cfg, "-p", "the secret prompt")
			require.Equal(t, 0, code, out)
			_, err := os.Stat(ran)
			assert.True(t, os.IsNotExist(err), "a refused prompt never reaches the agent")
			// the stream is JSON, so its newlines are escaped (recorded: snapshots/runs/prompt-blocked-json)
			want := `UserPromptSubmit operation blocked by hook:\nNOT-ALLOWED`
			if tc.wantPrompt {
				want += `\n\nOriginal prompt: the secret prompt`
			}
			assert.Contains(t, out, `"content":"`+want+`"`)
			assert.Contains(t, out, `"result":"`+want+`"`)
			// the frames and the transcript record say the same as for an exit 2
			var info, result map[string]any
			for _, l := range strings.Split(out, "\n") {
				var m map[string]any
				if json.Unmarshal([]byte(l), &m) != nil {
					continue
				}
				if m["subtype"] == "informational" {
					info = m
				} else if m["type"] == "result" {
					result = m
				}
			}
			require.NotNil(t, info)
			assert.Equal(t, "warning", info["level"])
			assert.Equal(t, true, info["prevent_continuation"])
			require.NotNil(t, result)
			assert.Equal(t, "success", result["subtype"])
			assert.Equal(t, false, result["is_error"])
			assert.EqualValues(t, 0, result["num_turns"])
			var rec map[string]any
			for _, r := range readRecs(t, transcriptPath(t, cfg, dir, "pj-1")) {
				if r.Type == "system" && r.Subtype == "informational" {
					require.NoError(t, json.Unmarshal([]byte(r.Raw), &rec))
				}
			}
			require.NotNil(t, rec, "the block is recorded in the transcript")
			assert.Equal(t, true, rec["preventContinuation"])
			assert.Contains(t, rec["content"], "NOT-ALLOWED")
		})
	}
}
