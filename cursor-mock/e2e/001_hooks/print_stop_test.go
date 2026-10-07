package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A10N_CURSOR_MOCK_STOP=1 is a deviation: cursor-agent -p never fires stop (runs/stop-block-continuation),
// but the TUI does (runs/tui-stop-followup, runs/tui-stop-followup-tools), and a test of what a
// stop hook does needs it in the mode a test drives. The mock then fires afterAgentResponse and
// stop as the TUI recordings show, and goes on with the hook's followup_message, printing nothing
// more on the stream.

// printStopMock runs the setup of the TUI run against the mock in print mode, the agent answering
// DONE, then DONE2 (after running `echo SECONDTURN` when tools is set) for the follow-up. It
// returns the shapes of the hooks that fired and the stream's frames.
func printStopMock(t *testing.T, run string, optIn, tools bool) (hooks, frames []map[string]any) {
	t.Helper()
	setup := filepath.Join(filepath.Dir(newestSample(t, run)), "..", "setup")
	ws, scratch, home := shortTempDir(t), t.TempDir(), t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	scripts, _ := filepath.Glob(filepath.Join(setup, "*.sh"))
	for _, s := range scripts {
		copyFile(t, s, filepath.Join(ws, ".cursor", "hooks", filepath.Base(s)), 0o755)
	}
	say := `printf '{"type":"assistant","message":{"content":[{"type":"text","text":"%s"}]}}\n{"type":"result","subtype":"success","is_error":false,"result":"%s"}\n' "$answer" "$answer"`
	body := "#!/bin/sh\nf=\"$A10N_MOCK_SESSION_FILE\"\nanswer=DONE\n[ \"$(grep -c '\"role\":\"user\"' \"$f\")\" -gt 1 ] && answer=DONE2\n"
	if tools {
		body += "if [ \"$answer\" = DONE2 ] && ! grep -q SECONDTURN \"$f\"; then\n" +
			`printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"echo SECONDTURN"}}]}}\n'; exit 0; fi` + "\n"
	}
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(body+say+"\n"), 0o755))
	log := filepath.Join(scratch, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--model", "auto", "--output-format", "stream-json", "go")
	cmd.Dir = ws
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log, "TMPDIR=" + scratch, "A10N_MOCK_SCRIPT=" + script}
	if optIn {
		cmd.Env = append(cmd.Env, "A10N_CURSOR_MOCK_STOP=1")
	}
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	require.NoError(t, cmd.Run(), errs.String())
	for _, l := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n")) {
		var f map[string]any
		require.NoError(t, json.Unmarshal(l, &f))
		frames = append(frames, f)
	}
	for _, m := range readJSONL(t, log) {
		if m["hook_event_name"] != nil && m["hook_event_name"] != "workspaceOpen" {
			hooks = append(hooks, tuiShape(m))
		}
	}
	return hooks, frames
}

// without drops the hooks the print mode does not fire as the TUI does (or the test does not
// compare): the prompt hook and, with tools, the tool hooks.
func without(hooks []map[string]any, drop ...string) (out []map[string]any) {
	for _, h := range hooks {
		skip := false
		for _, d := range drop {
			skip = skip || h["hook_event_name"] == d
		}
		if !skip {
			out = append(out, h)
		}
	}
	return out
}

// TestPrintModeFiresTheTuisStopOnlyWhenAskedTo: with the opt-in the hooks of a turn that a stop
// hook goes on with are the TUI's (runs/tui-stop-followup: response, stop, response, stop with
// loop_count 1, and no beforeSubmitPrompt, which print mode never fires), and the stream is the
// usual one, ending in one result. Without it print mode is as recorded: no stop.
func TestPrintModeFiresTheTuisStopOnlyWhenAskedTo(t *testing.T) {
	got, frames := printStopMock(t, "tui-stop-followup", true, false)
	want, _ := tuiRecorded(t, "tui-stop-followup")
	require.Equal(t, without(want, "beforeSubmitPrompt"), got)
	require.Equal(t, "result", frames[len(frames)-1]["type"])
	for _, f := range frames[:len(frames)-1] {
		require.NotEqual(t, "result", f["type"])
	}

	plain, _ := printStopMock(t, "tui-stop-followup", false, false)
	require.Equal(t, []string{"sessionStart", "sessionEnd"}, names(plain))
}

// TestPrintModeStopFollowUpCanUseTools: the follow-up turn runs a tool and then stops again, as
// recorded (runs/tui-stop-followup-tools).
func TestPrintModeStopFollowUpCanUseTools(t *testing.T) {
	toolHooks := []string{"beforeSubmitPrompt", "preToolUse", "postToolUse", "beforeShellExecution", "afterShellExecution"}
	got, _ := printStopMock(t, "tui-stop-followup-tools", true, true)
	want, _ := tuiRecorded(t, "tui-stop-followup-tools")
	require.Equal(t, without(want, toolHooks...), without(got, toolHooks...))
}
