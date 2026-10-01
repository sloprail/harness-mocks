package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_37_DurationExcludesPreToolUseHooks: duration_ms is the tool's own
// time (docs, PostToolUse input: "excludes time spent in permission prompts
// and PreToolUse hooks"), so a slow PreToolUse hook around a quick command
// does not show in it.
// sr:proves posttooluse-payload/claude
func TestT017_37_DurationExcludesPreToolUseHooks(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "post.log")
	pre := write(t, filepath.Join(dir, "pre.sh"), "#!/bin/sh\ncat >/dev/null\nsleep 1\n", 0o755)
	settings(t, dir, map[string]string{"PreToolUse": pre, "PostToolUse": payloadLogger(t, dir, "post.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`)),
		"--session-id", "dp-1", "--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Less(t, ps[0]["duration_ms"].(float64), 500.0, "the one-second PreToolUse hook is not part of the call's time")
}

// TestT017_40_AgentCallPostToolUsePayload: an Agent call fires PostToolUse as
// any tool does, with the call's input as given, its id and its duration (the
// hookerrors recording: tool_input {prompt, subagent_type, run_in_background},
// a tool_use_id and duration_ms); the duration covers the sub-agent's run.
// sr:proves posttooluse-payload/claude
func TestT017_40_AgentCallPostToolUsePayload(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "post.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "post.sh", log, "")})
	sub := write(t, filepath.Join(dir, "sub.sh"), "#!/bin/sh\nsleep 0.3\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"HELPED\"}'\n", 0o755)
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"Reply HELPED","description":"d","subagent_type":"general-purpose","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "ap-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	p := ps[0]
	assert.Equal(t, "Agent", p["tool_name"])
	in := p["tool_input"].(map[string]any)
	assert.Equal(t, "Reply HELPED", in["prompt"])
	assert.Equal(t, "general-purpose", in["subagent_type"])
	assert.True(t, strings.HasPrefix(p["tool_use_id"].(string), "ag1"))
	assert.GreaterOrEqual(t, p["duration_ms"].(float64), 250.0, "the call lasts as long as the sub-agent's run")
}

// TestT017_41_SessionStartContextFromSeveralHooksWhole: several SessionStart
// hooks each add context, all of it kept in order, whatever its length; the
// agent's scenario receives it, and the mock does not cut it short (declared
// deviation: Claude Code moves context over 10,000 characters to a file).
// sr:proves hook-additional-context/claude
// sr:proves hooks-all-matching-run/claude
func TestT017_41_SessionStartContextFromSeveralHooksWhole(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	long := strings.Repeat("x", 12000)
	hook := func(name, text string) string {
		return write(t, filepath.Join(dir, name), "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"hookSpecificOutput\":{\"hookEventName\":\"SessionStart\",\"additionalContext\":\""+text+"\"}}'\n", 0o755)
	}
	h1, h2 := hook("h1.sh", "FIRST"), hook("h2.sh", long)
	write(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"command","command":"`+h1+`"},{"type":"command","command":"`+h2+`"}]}]}}`, 0o644)
	sc := write(t, filepath.Join(dir, "s.sh"), "#!/bin/sh\nprintf %s \"$A10N_MOCK_ADDITIONAL_CONTEXT\" > "+filepath.Join(dir, "ctx.out")+"\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"done\"}'\n", 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "sc-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	got, err := os.ReadFile(filepath.Join(dir, "ctx.out"))
	require.NoError(t, err)
	assert.Equal(t, "FIRST\n"+long, string(got))
}

// TestT017_38_SessionHooksCannotStopTheSession: a SessionStart hook that tries
// to stop the session (a JSON block, continue:false, exit 2) does not: the
// agent runs and the session ends in its own time. A SessionEnd hook's JSON
// output is discarded (docs: SessionEnd has no decision control), and SessionEnd
// reports reason "other" after the last Stop, in the stream-json session the
// recordings are of.
// sr:proves session-start-hook/claude
// sr:proves session-end-hook/claude
func TestT017_38_SessionHooksCannotStopTheSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	ran := filepath.Join(dir, "ran")
	start := write(t, filepath.Join(dir, "start.sh"), `#!/bin/sh
cat >/dev/null
echo '{"decision":"block","reason":"STOP-NOW","continue":false,"stopReason":"STOP-NOW"}'
exit 2
`, 0o755)
	end := payloadLogger(t, dir, "end.sh", log, `echo '{"decision":"block","reason":"X","continue":false,"systemMessage":"SYSMSG"}'`)
	stop := payloadLogger(t, dir, "stop.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": start, "Stop": stop, "SessionEnd": end})
	sc := write(t, filepath.Join(dir, "s.sh"), "#!/bin/sh\n: > "+ran+"\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"done\"}'\n", 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "sh-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	_, err := os.Stat(ran)
	assert.NoError(t, err, "a start hook cannot stop the session from starting")
	assert.NotContains(t, out, "SYSMSG", "SessionEnd's JSON output fields are discarded")

	var order []string
	for _, p := range payloads(t, log) {
		order = append(order, p["hook_event_name"].(string))
		if p["hook_event_name"] == "SessionEnd" {
			assert.Equal(t, "other", p["reason"])
		}
	}
	assert.Equal(t, []string{"Stop", "SessionEnd"}, order)
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "sh-1"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "SYSMSG", "what a SessionEnd hook prints is not in the transcript")
}

// TestT017_39_StopPayloadMessageFollowsTheTurn: each Stop carries the text of
// that turn's final response: after a block the agent answers again and the
// next Stop carries the new answer (the stops recording: DONE, then DONE2),
// with stop_hook_active true for it.
// sr:proves stop-hook-payload/claude
func TestT017_39_StopPayloadMessageFollowsTheTurn(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "stop.log")
	counter := filepath.Join(dir, "n")
	hook := payloadLogger(t, dir, "stop.sh", log, `N=$(cat `+counter+` 2>/dev/null || echo 0); N=$((N+1)); echo $N > `+counter+`
if [ $N = 1 ]; then echo '{"decision":"block","reason":"again"}'; fi`)
	settings(t, dir, map[string]string{"Stop": hook})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
if grep -q 'Stop hook feedback' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then T=SECOND; else T=FIRST; fi
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"'$T'"}]}}'
echo '{"type":"result","subtype":"success","result":"'$T'"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "sm-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	var msgs []string
	var active []any
	for _, p := range payloads(t, log) {
		msgs = append(msgs, p["last_assistant_message"].(string))
		active = append(active, p["stop_hook_active"])
	}
	assert.Equal(t, []string{"FIRST", "SECOND"}, msgs)
	assert.Equal(t, []any{false, true}, active)
	assert.True(t, strings.Contains(out, "SECOND"))
}
