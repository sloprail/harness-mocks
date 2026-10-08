package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The TUI runs were recorded by tools/tui-record driving the pinned cursor-agent without -p
// (runs/tui-*); they have no stream, so what is compared is the hook payloads, in order, field by
// field of the ones the recordings pin.

// tuiShape is a hook payload as the recordings pin it.
func tuiShape(h map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"hook_event_name", "prompt", "text", "status", "loop_count", "trigger", "is_first_compaction", "reason", "final_status"} {
		if v, ok := h[k]; ok {
			out[k] = v
		}
	}
	return out
}

// tuiRecorded are the shapes of the hooks the run's newest sample recorded, and the hook_results
// its hook script logged.
func tuiRecorded(t *testing.T, run string) (shapes, results []map[string]any) {
	t.Helper()
	for _, e := range readJSONL(t, filepath.Join(newestSample(t, run), "events.jsonl")) {
		p, _ := e["payload"].(map[string]any)
		if r, ok := p["hook_result"].(map[string]any); ok {
			results = append(results, r)
		} else if e["hook"] != nil {
			shapes = append(shapes, tuiShape(p))
		}
	}
	return shapes, results
}

// tuiMock plays the run's setup against the mock started without -p, the user's typing on its
// stdin; the agent answers DONE, then DONE2 for a follow-up.
func tuiMock(t *testing.T, run, typed string) (shapes, results []map[string]any) {
	t.Helper()
	shapes, results, _ = tuiMockWith(t, run, typed, "")
	return shapes, results
}

// tuiMockWith is tuiMock with the run's hook script replaced by hook when it is not empty. It also
// returns the scratch directory, where session.last holds the session transcript as the agent
// saw it when its last generation began.
func tuiMockWith(t *testing.T, run, typed, hook string) (shapes, results []map[string]any, scratch string) {
	t.Helper()
	setup := filepath.Join(filepath.Dir(newestSample(t, run)), "..", "setup")
	ws, home := shortTempDir(t), t.TempDir()
	scratch = t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	scripts, _ := filepath.Glob(filepath.Join(setup, "*.sh"))
	for _, s := range scripts {
		copyFile(t, s, filepath.Join(ws, ".cursor", "hooks", filepath.Base(s)), 0o755)
	}
	if hook != "" {
		require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks", "hook.sh"), []byte(hook), 0o755))
	}
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncp \"$A10N_MOCK_SESSION_FILE\" \"$TMPDIR/session.last\"\nanswer=DONE\n[ \"$(grep -c '\"role\":\"user\"' \"$A10N_MOCK_SESSION_FILE\")\" -gt 1 ] && answer=DONE2\n"+
		"printf '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"%s\"}]}}\\n{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"%s\"}\\n' \"$answer\" \"$answer\"\n"), 0o755))
	log := filepath.Join(scratch, "payloads.jsonl")
	cmd := exec.Command(binary, "--trust", "--model", "auto")
	cmd.Dir, cmd.Stdin = ws, strings.NewReader(typed)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log, "TMPDIR=" + scratch, "A10N_MOCK_SCRIPT=" + script}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the mock failed: %s", out)
	for _, m := range readJSONL(t, log) {
		if r, ok := m["hook_result"].(map[string]any); ok {
			results = append(results, r)
		} else if m["hook_event_name"] != nil {
			shapes = append(shapes, tuiShape(m))
		}
	}
	return shapes, results, scratch
}

func names(shapes []map[string]any) (out []string) {
	for _, s := range shapes {
		out = append(out, s["hook_event_name"].(string))
	}
	return out
}

// TestTuiTurnEndsWithAStopHookThatFollowsTheResponse: recorded (runs/tui-stop, cursor-agent without
// -p), the TUI fires beforeSubmitPrompt with the prompt, then afterAgentResponse with the agent's
// message, then stop with status completed and loop_count 0 (no last message: that is the
// response hook's), and no stream or result: the mock fires the same.
func TestTuiTurnEndsWithAStopHookThatFollowsTheResponse(t *testing.T) {
	want, _ := tuiRecorded(t, "tui-stop")
	got, _ := tuiMock(t, "tui-stop", "Reply only DONE.\n")
	require.Equal(t, want, got)
	require.Equal(t, []string{"sessionStart", "beforeSubmitPrompt", "afterAgentResponse", "stop", "sessionEnd"}, names(got))
	require.Equal(t, "DONE", got[2]["text"])
	require.Equal(t, float64(0), got[3]["loop_count"])
	require.NotContains(t, got[3], "text")
}

// TestTuiStopHookFollowupGoesOnWithTheTurn: recorded (runs/tui-stop-followup), a stop hook that
// answers {"followup_message": ...} has it taken as the agent's next message, with no
// beforeSubmitPrompt, and the second stop says loop_count 1; the mock does the same.
// sr:proves stop-block-continuation/cursor
func TestTuiStopHookFollowupGoesOnWithTheTurn(t *testing.T) {
	want, _ := tuiRecorded(t, "tui-stop-followup")
	got, _ := tuiMock(t, "tui-stop-followup", "Reply only DONE.\n")
	require.Equal(t, want, got)
	require.Equal(t, []string{"sessionStart", "beforeSubmitPrompt", "afterAgentResponse", "stop", "afterAgentResponse", "stop", "sessionEnd"}, names(got))
	require.Equal(t, "DONE2", got[4]["text"])
	require.Equal(t, float64(1), got[5]["loop_count"])
}

// TestTuiStopHookReasonIsHandedToTheAgentAsItsNextMessage: the text of the stop hook's
// followup_message is what the agent is given as its next user message, after the typed prompt
// (runs/tui-stop-followup: the hook answered "STOP-FOLLOWUP-REASON: reply only DONE2.").
// sr:proves stop-block-continuation/cursor
func TestTuiStopHookReasonIsHandedToTheAgentAsItsNextMessage(t *testing.T) {
	_, _, scratch := tuiMockWith(t, "tui-stop-followup", "Reply only DONE.\n", "")
	var users []string
	for _, r := range readJSONL(t, filepath.Join(scratch, "session.last")) {
		if r["role"] == "user" {
			b, err := json.Marshal(r)
			require.NoError(t, err)
			users = append(users, string(b))
		}
	}
	require.Len(t, users, 2)
	require.Contains(t, users[0], "Reply only DONE.")
	require.Contains(t, users[1], "STOP-FOLLOWUP-REASON: reply only DONE2.")
}

// TestTuiStopHookThatAlwaysBlocksIsOverriddenAfterFiveFollowUps: the docs' loop_limit (default 5,
// hooks#per-script-configuration-options) ends the continuation: a stop hook answering a
// followup_message every time is followed up five times, its sixth stop says loop_count 5, and
// the turn ends there. The cap is the docs' (no recording reaches it).
func TestTuiStopHookThatAlwaysBlocksIsOverriddenAfterFiveFollowUps(t *testing.T) {
	hook := "#!/bin/sh\nIN=$(cat)\nprintf '%s\\n' \"$IN\" >>\"$HOOK_LOG\"\n" +
		"[ \"$(printf '%s' \"$IN\" | jq -r .hook_event_name)\" = stop ] && echo '{\"followup_message\":\"again\"}'\nexit 0\n"
	got, _, _ := tuiMockWith(t, "tui-stop-followup", "Reply only DONE.\n", hook)
	var loops []float64
	for _, h := range got {
		if h["hook_event_name"] == "stop" {
			loops = append(loops, h["loop_count"].(float64))
		}
	}
	require.Equal(t, []float64{0, 1, 2, 3, 4, 5}, loops)
}

// TestTuiPromptRefusedByAHookNeverReachesTheAgent: recorded (runs/tui-prompt-blocked), a
// beforeSubmitPrompt hook answering {"continue": false} stops the prompt: no response, no stop,
// the session ends; the mock does the same.
func TestTuiPromptRefusedByAHookNeverReachesTheAgent(t *testing.T) {
	want, _ := tuiRecorded(t, "tui-prompt-blocked")
	got, _ := tuiMock(t, "tui-prompt-blocked", "Reply only DONE.\n")
	require.Equal(t, want, got)
	require.Equal(t, []string{"sessionStart", "beforeSubmitPrompt", "sessionEnd"}, names(got))
}

// TestTuiCompressCompactsWithoutTouchingTheTranscript: recorded (runs/tui-manual-compaction),
// /compress typed at the idle input fires preCompact with trigger "manual", then a response with
// no text and a stop, and the transcript of the session still begins with every record it held
// at preCompact, byte for byte, in the same file; the mock does the same. (Not a proof of the
// manual-compaction or compaction-transcript-continuity cells, which Cursor does not meet: its
// compaction cannot be stopped by the hook, and writes no boundary or summary.)
func TestTuiCompressCompactsWithoutTouchingTheTranscript(t *testing.T) {
	want, wantResults := tuiRecorded(t, "tui-manual-compaction")
	got, gotResults := tuiMock(t, "tui-manual-compaction", "Reply only DONE.\n/compress\n")
	require.Equal(t, want, got)
	require.Equal(t, "manual", got[4]["trigger"])
	require.Equal(t, true, got[4]["is_first_compaction"])
	require.Equal(t, []map[string]any{{"event": "sessionEnd", "prefix_kept": true, "same_file": true}}, wantResults)
	require.Equal(t, wantResults, gotResults)
}
