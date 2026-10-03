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

// The recorded run runs/noninteractive-no-force: cursor-agent -p --trust (no
// --force or --yolo) given a prompt that writes a file, runs a command and
// reads the file back. The headless doc says that without --force changes are
// only proposed and files stay as they were; what was recorded is that the
// write was applied and read back, and the command was rejected without being
// run (its stream result a rejection, from the workspace, with no reason).

// rejectionsOf are the rejected objects of the completed tool-call frames of a
// stream, as printed, with the workspace as <RUN>.
func rejectionsOf(frames []map[string]any, ws string) (out []map[string]any) {
	for _, f := range frames {
		if f["type"] != "tool_call" || f["subtype"] != "completed" {
			continue
		}
		for _, v := range f["tool_call"].(map[string]any) {
			body, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if res, ok := body["result"].(map[string]any); ok {
				if rej, ok := res["rejected"].(map[string]any); ok {
					named := map[string]any{}
					for k, v := range rej {
						if s, ok := v.(string); ok && ws != "" {
							v = strings.ReplaceAll(s, ws, "<RUN>")
						}
						named[k] = v
					}
					out = append(out, named)
				}
			}
		}
	}
	return out
}

// TestWithoutForceACommandIsRejectedUnrunAndAFileWriteIsApplied: recorded, a run
// without --force or --yolo applies the agent's write (the sessionEnd hook
// finds the file, and the read of it returns its content), runs no command: its
// preToolUse and beforeShellExecution hooks fire, then the stream's result is a
// rejection naming the command and the workspace, with an empty reason and
// isReadonly false, and the hooks after it report what a command that printed
// nothing gives them (no output, exit 0). The mock's frames and hook payloads
// are the recorded ones.
// sr:proves noninteractive-run/cursor
func TestWithoutForceACommandIsRejectedUnrunAndAFileWriteIsApplied(t *testing.T) {
	got, want := replay(t, "noninteractive-no-force")
	conforms(t, got, want)

	require.Equal(t, []string{
		"tool_call/started/editToolCall/", "tool_call/completed/editToolCall/success",
		"tool_call/started/shellToolCall/", "tool_call/completed/shellToolCall/rejected",
		"tool_call/started/readToolCall/", "tool_call/completed/readToolCall/success",
		"result/success",
	}, want.frames, "recorded")
	require.Equal(t, want.frames, got.frames)

	// the write was applied, in the recording and in the mock
	for _, m := range readJSONL(t, filepath.Join(newestSample(t, "noninteractive-no-force"), "payloads.jsonl")) {
		if r, ok := m["hook_result"].(map[string]any); ok && r["event"] == "files" {
			require.Equal(t, true, r["note_exists"])
			require.Equal(t, "hi", r["note"])
		}
	}
	b, err := os.ReadFile(filepath.Join(got.ws, "note.txt"))
	require.NoError(t, err)
	require.Equal(t, "hi\n", string(b))

	// the command did not run: what its after-hooks are given is nothing printed and exit 0
	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		require.Equal(t, "preToolUse beforeShellExecution afterShellExecution postToolUse", joined(eventsOf(o, "echo FINE")), name)
		for _, h := range o.hooks {
			if h["hook_event_name"] == "afterShellExecution" {
				require.Equal(t, "", h["output"], name)
			}
			if h["hook_event_name"] == "postToolUse" && h["tool_name"] == "Shell" {
				require.JSONEq(t, `{"output":"","exitCode":0}`, h["tool_output"].(string), name)
			}
		}
	}

	// the rejection's own object, as recorded
	recorded := rejectionsOf(readJSONL(t, filepath.Join(newestSample(t, "noninteractive-no-force"), "stream.jsonl")), "")
	var printed []map[string]any
	for _, l := range strings.Split(got.stdout, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			printed = append(printed, f)
		}
	}
	mock := rejectionsOf(printed, got.ws)
	require.Len(t, recorded, 1)
	require.Equal(t, map[string]any{"command": "echo FINE", "workingDirectory": "<RUN>", "reason": "", "isReadonly": false}, recorded[0])
	require.Equal(t, recorded, mock)
}

// forcedRun plays the one command through the mock with the given flags, and
// returns the stream it printed.
func forcedRun(t *testing.T, flags []string, command string) string {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	call := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_0","name":"Bash","input":{"command":` + jsonString(command) + `}}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "0.json"), []byte(call+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "end.json"), []byte(`{"type":"result","subtype":"success","result":"DONE"}`+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\nf=\""+scratch+"/$n.json\"\n[ -f \"$f\" ] || f=\""+scratch+"/end.json\"\ncat \"$f\"\n"), 0o755))
	args := append(append([]string{"-p"}, flags...), "--output-format", "stream-json", "--script", script, "go")
	cmd := exec.Command(binary, args...)
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

// TestForceOrYoloApprovesShellCommands: the headless doc names --force and
// --yolo as the way to let a print-mode run act without confirmation, and the
// recording shows the command rejected without them: with either flag the
// command runs and its result is a success carrying its output, and without
// both it is rejected and prints nothing.
// sr:proves noninteractive-run/cursor
func TestForceOrYoloApprovesShellCommands(t *testing.T) {
	for _, flags := range [][]string{{"--force"}, {"-f"}, {"--yolo"}, {"--trust", "--force"}} {
		out := forcedRun(t, flags, "echo APPROVED")
		require.Contains(t, out, `"stdout":"APPROVED\n"`, strings.Join(flags, " "))
		require.Contains(t, out, `"success"`, strings.Join(flags, " "))
		require.NotContains(t, out, `"rejected"`, strings.Join(flags, " "))
	}
	for _, flags := range [][]string{nil, {"--trust"}} {
		out := forcedRun(t, flags, "echo APPROVED")
		require.Contains(t, out, `"rejected"`, strings.Join(flags, " "))
		require.NotContains(t, out, "APPROVED\\n", "the command did not run")
	}
}
