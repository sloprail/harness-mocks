package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logHook appends every payload to $HOOK_LOG, one line each.
const logHook = `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"`

func eventsOf(r result, name string) []map[string]any {
	var out []map[string]any
	for _, l := range r.hookLog() {
		if l["hook_event_name"] == name {
			out = append(out, l)
		}
	}
	return out
}

// Context a hook adds (SessionStart, UserPromptSubmit) reaches the agent as its
// own role=developer message holding exactly that text, whether the hook
// printed plain text or JSON additionalContext (runs/stops: SS-CTX by JSON;
// runs/hook-exit-codes: the secret word by plain text). Plain text printed by
// a PostToolUse hook is ignored (hooks#posttooluse); its JSON additionalContext
// is added. When several hooks add text all of it is kept.
// sr:proves hook-additional-context/codex
func TestHookAddedContextIsKept(t *testing.T) {
	plain := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "SessionStart", "UserPromptSubmit", "PostToolUse"),
		Files: map[string]string{"hook.sh": `in=$(cat); case "$in" in
  *SessionStart*) echo "CTX-START" ;;
  *UserPromptSubmit*) echo '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"CTX-PROMPT"}}' ;;
  *PostToolUse*) echo "POST-PLAIN" ;;
esac`},
		Script: callThenResult, Prompt: "go", Env: withCalls(t, "true"),
	})
	assert.Equal(t, []string{"CTX-START", "CTX-PROMPT"}, developerTexts(t, plain.rollout(t)))
	assert.NotContains(t, plain.rollout(t), "POST-PLAIN", "plain text from a PostToolUse hook was kept")

	byJSON := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "SessionStart", "PostToolUse"),
		Files: map[string]string{"hook.sh": `in=$(cat); case "$in" in
  *SessionStart*) echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"CTX-START"}}' ;;
  *PostToolUse*) echo '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"CTX-POST"}}' ;;
esac`},
		Script: callThenResult, Prompt: "go", Env: withCalls(t, "true"),
	})
	assert.Equal(t, []string{"CTX-START", "CTX-POST"}, developerTexts(t, byJSON.rollout(t)))

	two := execMock(t, scenario{
		HooksJSON: `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"echo FIRST"},{"type":"command","command":"echo SECOND"}]}]}}`,
		Script:    callThenResult, Prompt: "go", Env: withCalls(t),
	})
	joined := strings.Join(developerTexts(t, two.rollout(t)), "\n")
	assert.Contains(t, joined, "FIRST")
	assert.Contains(t, joined, "SECOND")
}

// A command hook given as a shell line runs through the shell in the session's
// directory with the event payload as JSON on stdin.
// sr:proves hook-command-handler/codex
func TestCommandHookReadsThePayloadOnStdinInTheSessionDirectory(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON(`pwd -P >>"$TMPDIR/dirs"; cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"; true | true`, "SessionStart", "PreToolUse", "PostToolUse"),
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo hi"),
	})
	want, err := filepath.EvalSymlinks(r.Repo)
	require.NoError(t, err)
	// Every hook ran in exactly the session directory (not a subdirectory of it).
	assert.Equal(t, []string{want, want, want}, strings.Fields(readFile(t, filepath.Join(r.Tmp, "dirs"))))

	sid := sessionIDOf(t, r)
	start := eventsOf(r, "SessionStart")
	require.Len(t, start, 1)
	assert.Equal(t, "startup", start[0]["source"])
	pre, post := eventsOf(r, "PreToolUse"), eventsOf(r, "PostToolUse")
	require.Len(t, pre, 1)
	require.Len(t, post, 1)
	for _, p := range []map[string]any{start[0], pre[0], post[0]} {
		assert.Equal(t, sid, p["session_id"], p["hook_event_name"])
		cwd, err := filepath.EvalSymlinks(p["cwd"].(string))
		require.NoError(t, err)
		assert.Equal(t, want, cwd, p["hook_event_name"])
	}
	for _, p := range []map[string]any{pre[0], post[0]} {
		assert.NotEmpty(t, p["tool_name"], p["hook_event_name"])
		assert.Contains(t, p["tool_input"], "command", p["hook_event_name"])
		assert.Contains(t, p["tool_input"].(map[string]any)["command"], "echo hi", p["hook_event_name"])
	}
}

// Every hook payload names the session, its transcript file and its working
// directory; the main thread names no sub-agent (runs/*: session_id,
// transcript_path, cwd on every event).
// sr:proves hook-common-payload/codex
func TestEveryPayloadNamesSessionTranscriptAndDirectory(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "SessionEnd"),
		Files:     map[string]string{"hook.sh": logHook},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "true"),
	})
	log := r.hookLog()
	require.Len(t, log, 6)
	sid := sessionIDOf(t, r)
	for _, p := range log {
		assert.Equal(t, sid, p["session_id"], p["hook_event_name"])
		assert.Equal(t, r.Repo, p["cwd"])
		tp, _ := p["transcript_path"].(string)
		assert.True(t, strings.HasPrefix(tp, r.Home) && strings.Contains(tp, sid), "transcript_path %q", tp)
		assert.NotContains(t, p, "agent_id")
	}
}

// Hooks run only for events whose subject matches (runs/hook-matchers): "*",
// "" and a matching regular expression run, a non-matching one does not, and
// events without a subject (prompt, stop) take every hook whatever its matcher.
// sr:proves hook-matcher-filter/codex
func TestMatchersSelectHooks(t *testing.T) {
	rec := loadRecording(t, "hook-matchers")
	r := replay(t, rec)
	ran := map[string]bool{}
	for _, l := range r.hookLog() {
		ran[l["ran"].(string)] = true
	}
	for _, name := range []string{"pre-bash", "pre-star", "pre-empty", "post-bash", "session-startup", "prompt-ignored-matcher", "stop-ignored-matcher"} {
		assert.True(t, ran[name], "%s did not run", name)
	}
	for _, name := range []string{"pre-edit-write", "pre-nomatch", "post-patch", "session-resume"} {
		assert.False(t, ran[name], "%s ran", name)
	}
}

// A hook that outlives its timeout is killed with everything it spawned, what
// it printed (a deny) is discarded, and the event carries on (runs/hook-timeout).
// sr:proves hook-timeout/codex
func TestHookTimeoutKillsTheGroupDiscardsOutputAndGoesOn(t *testing.T) {
	r := replay(t, loadRecording(t, "hook-timeout"))
	cmds, _ := r.commands()
	assert.Equal(t, []string{"echo one"}, cmds, "the discarded deny refused the call")
	var gone bool
	for _, l := range r.hookLog() {
		gone = gone || l["grandchild"] == "gone"
	}
	assert.True(t, gone, "a process the hook spawned outlived its timeout")
}

// A prompt runs to completion without interaction: one event per record, and
// a single end (runs/*: thread.started, turn.started, items, turn.completed).
// sr:proves noninteractive-run/codex
func TestNonInteractiveRunStreamsEventsAndEndsOnce(t *testing.T) {
	r := execMock(t, scenario{Script: callThenResult, Prompt: "go", Env: withCalls(t, "echo hi")})
	require.Equal(t, 0, r.Code, r.Stderr)
	assert.Equal(t, []string{"thread.started", "turn.started", "item.started:command_execution",
		"item.completed:command_execution", "turn.completed"}, streamShape(r.stream()))
	var types []string
	for _, e := range r.stream() {
		types = append(types, e["type"].(string))
	}
	assert.Equal(t, 1, strings.Count(strings.Join(types, " "), "turn.completed"))
	assert.Equal(t, "thread.started", types[0])
}

// The session-end hook fires once the turn is over, with the one reason a
// non-interactive run has, and names neither model nor permission mode; what it
// prints is not read (runs/session-end).
// sr:proves session-end-hook/codex
func TestSessionEndFiresLastWithTheGenericReason(t *testing.T) {
	rec := loadRecording(t, "session-end")
	r := replay(t, rec)
	log := r.hookLog()
	last := log[len(log)-1]
	assert.Equal(t, "SessionEnd", last["hook_event_name"])
	assert.Equal(t, "other", last["reason"])
	assert.NotContains(t, last, "model")
	assert.NotContains(t, last, "permission_mode")
	assert.NotContains(t, r.rollout(t), `"reason":"other"`)
}

// The session-start hook fires when a session begins and says how: a fresh run
// is "startup". A hook that exits 2 does not stop the session (runs/hook-exit-codes).
// sr:proves session-start-hook/codex
func TestSessionStartSaysStartupAndCannotStopTheSession(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "SessionStart"),
		Files:     map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"; echo no >&2; exit 2`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo ran"),
	})
	assert.Equal(t, "startup", eventsOf(r, "SessionStart")[0]["source"])
	cmds, _ := r.commands()
	assert.Equal(t, []string{"echo ran"}, cmds)
}

// When a stop hook blocks, its reason is handed back to the agent, the turn
// goes on, and the caller sees a single end (runs/stops: two blocks, one turn).
// sr:proves stop-block-continuation/codex
func TestStopBlockContinuesTheTurnWithOneEnd(t *testing.T) {
	rec := loadRecording(t, "stops")
	r := replay(t, rec)
	assert.Len(t, eventsOf(r, "Stop"), 3)
	assert.Contains(t, r.rollout(t), "BLOCK-JSON-REASON")
	assert.Contains(t, r.rollout(t), "BLOCK-EXIT2-REASON")
	n := 0
	for _, e := range r.stream() {
		if e["type"] == "turn.completed" {
			n++
		}
	}
	assert.Equal(t, 1, n)
}

// The stop payload carries the agent's last message and whether the turn is
// already continuing because an earlier hook blocked (runs/stops).
// sr:proves stop-hook-payload/codex
func TestStopPayloadCarriesLastMessageAndContinuing(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "Stop"),
		Files:     map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"; if [ ! -f "$TMPDIR/once" ]; then : >"$TMPDIR/once"; echo again >&2; exit 2; fi`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t),
	})
	stops := eventsOf(r, "Stop")
	require.Len(t, stops, 2)
	assert.Equal(t, false, stops[0]["stop_hook_active"])
	assert.Equal(t, true, stops[1]["stop_hook_active"])
	assert.Equal(t, "DONE", stops[0]["last_assistant_message"])
}

// After a tool call, a hook fires with the tool's input and its response
// (runs/stops: tool_input.command and tool_response).
// sr:proves posttooluse-payload/codex
func TestPostToolUsePayloadCarriesInputAndResponse(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PostToolUse"),
		Files:     map[string]string{"hook.sh": logHook},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo FINE"),
	})
	post := eventsOf(r, "PostToolUse")
	require.Len(t, post, 1)
	assert.Equal(t, "Bash", post[0]["tool_name"])
	assert.Equal(t, map[string]any{"command": "echo FINE"}, post[0]["tool_input"])
	assert.Equal(t, "FINE\n", post[0]["tool_response"])
}

// A hook fires before a submitted prompt reaches the agent: it can add context
// (plain text) or refuse the prompt (exit 2), and the prompt reaches the script
// unchanged (runs/hook-exit-codes, runs/prompt-blocked).
// sr:proves user-prompt-submit-hook/codex
func TestUserPromptSubmitAddsContextOrRefuses(t *testing.T) {
	script := "echo \"prompt=[$A10N_MOCK_PROMPT] ctx=[$A10N_MOCK_ADDITIONAL_CONTEXT]\" >>\"$TMPDIR/seen\"\n" + callThenResult
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "UserPromptSubmit"),
		Files:     map[string]string{"hook.sh": `in=$(cat); printf '%s\n' "$in" >>"$HOOK_LOG"; echo "BANANA"`},
		Script:    script, Prompt: "the prompt", Env: withCalls(t),
	})
	assert.Equal(t, "the prompt", eventsOf(r, "UserPromptSubmit")[0]["prompt"])
	assert.Contains(t, readFile(t, filepath.Join(r.Tmp, "seen")), "prompt=[the prompt] ctx=[BANANA]")

	refused := replay(t, loadRecording(t, "prompt-blocked"))
	assert.Equal(t, []string{"thread.started", "turn.started", "turn.completed"}, streamShape(refused.stream()))
}
