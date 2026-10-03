package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/pretool-refusal-file-tools: a preToolUse hook refuses
// the Write of protected.txt (which the session's start made, holding
// ORIGINAL) by a JSON deny and the Read of secret.txt by exit 2; the command
// after them runs.

// toolRun is a run of the mock whose script makes the tool calls it is given,
// each as the scenario protocol's tool_use block (name and input).
type toolRun struct {
	ws, log string
	frames  []map[string]any
}

func runTools(t *testing.T, hooksJSON string, scripts map[string]string, files map[string]string, calls ...map[string]any) toolRun {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".cursor", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks.json"), []byte(hooksJSON), 0o644))
	for name, body := range scripts {
		require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks", name), []byte(body), 0o755))
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(ws, name), []byte(body), 0o644))
	}
	for i, c := range calls {
		block := map[string]any{"type": "tool_use", "id": "tu_" + strconv.Itoa(i), "name": c["name"], "input": c["input"]}
		line := jsonString(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{block}}})
		require.NoError(t, os.WriteFile(filepath.Join(scratch, strconv.Itoa(i)+".json"), []byte(line+"\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "end.json"), []byte(`{"type":"result","subtype":"success","result":"DONE"}`+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\nf=\""+scratch+"/$n.json\"\n[ -f \"$f\" ] || f=\""+scratch+"/end.json\"\ncat \"$f\"\n"), 0o755))
	logPath := filepath.Join(scratch, "log.txt")
	cmd := exec.Command(binary, "-p", "--force", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "HOOK_LOG=" + logPath}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	r := toolRun{ws: ws, log: logPath}
	for _, l := range strings.Split(string(out), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			r.frames = append(r.frames, f)
		}
	}
	return r
}

// completed is the result object of the completed frame of the n-th call of a
// kind (editToolCall, readToolCall), or nil.
func (r toolRun) completed(kind string, n int) map[string]any {
	for _, f := range r.frames {
		if f["type"] != "tool_call" || f["subtype"] != "completed" {
			continue
		}
		if call, ok := f["tool_call"].(map[string]any)[kind].(map[string]any); ok {
			if n == 0 {
				res, _ := call["result"].(map[string]any)
				return res
			}
			n--
		}
	}
	return nil
}

func (r toolRun) logged(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(r.log)
	return string(b)
}

// TestAPreToolUseDenyOfAWriteRefusesItAndLeavesTheFileUntouched: recorded, a
// preToolUse hook that denies the Write of a file by JSON refuses the call:
// the file still holds what it held at the start (the sessionEnd hook logs it
// as ORIGINAL), no afterFileEdit or postToolUse fires for the call, the
// failure hook reports permission_denied with the hook's user_message, and the
// write's result to the agent is an error, not a rejection, carrying that
// message and the note not to look for workarounds. The refused call is the
// agent's own write: the read it makes of the file first is a call of its own
// that the hook allowed.
// sr:proves pretooluse-refusal/cursor
func TestAPreToolUseDenyOfAWriteRefusesItAndLeavesTheFileUntouched(t *testing.T) {
	got, want := replay(t, "pretool-refusal-file-tools")
	conforms(t, got, want)

	recorded := readJSONL(t, filepath.Join(newestSample(t, "pretool-refusal-file-tools"), "payloads.jsonl"))
	var files []map[string]any
	for _, m := range recorded {
		if r, ok := m["hook_result"].(map[string]any); ok && r["event"] == "files" {
			files = append(files, r)
		}
	}
	require.Len(t, files, 1)
	require.Equal(t, "ORIGINAL", files[0]["protected"], "recorded: the refused write left the file as it was")
	b, err := os.ReadFile(filepath.Join(got.ws, "protected.txt"))
	require.NoError(t, err)
	require.Equal(t, "ORIGINAL\n", string(b), "mock: the refused write left the file as it was")

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		var writes []string
		for _, h := range o.hooks {
			if in, ok := h["tool_input"].(map[string]any); ok && h["tool_name"] == "Write" {
				require.Equal(t, "<RUN>/protected.txt", in["file_path"], name)
				writes = append(writes, h["hook_event_name"].(string))
				if h["hook_event_name"] == "postToolUseFailure" {
					require.Equal(t, "WRITE-DENY-MSG", h["error_message"], name)
					require.Equal(t, "permission_denied", h["failure_type"], name)
				}
			}
			require.NotEqual(t, "afterFileEdit", h["hook_event_name"], name+": the refused write edited nothing")
		}
		require.Equal(t, []string{"preToolUse", "postToolUseFailure"}, writes, name+": the call that was refused has no postToolUse")
		require.Contains(t, o.frames, "tool_call/completed/editToolCall/error", name)
		require.NotContains(t, o.frames, "tool_call/completed/editToolCall/success", name)
	}

	// the frame's own shape, as recorded: an error naming the hook's message
	deny := `#!/bin/sh
cat >/dev/null
echo '{"permission":"deny","user_message":"WRITE-DENY-MSG"}'
`
	r := runTools(t, `{"version":1,"hooks":{"preToolUse":[{"command":".cursor/hooks/deny.sh","matcher":"Write"}],"afterFileEdit":[{"command":"cat >> \"$HOOK_LOG\""}]}}`,
		map[string]string{"deny.sh": deny}, map[string]string{"protected.txt": "ORIGINAL\n"},
		map[string]any{"name": "Write", "input": map[string]any{"file_path": "protected.txt", "content": "CHANGED\n"}})
	res := r.completed("editToolCall", 0)
	require.NotContains(t, res, "success")
	e, _ := res["error"].(map[string]any)
	const msg = "WRITE-DENY-MSG\n\nAgent note: Do not suggest workarounds to the blocked tool."
	require.Equal(t, msg, e["error"])
	require.Equal(t, msg, e["modelVisibleError"])
	content, err := os.ReadFile(filepath.Join(r.ws, "protected.txt"))
	require.NoError(t, err)
	require.Equal(t, "ORIGINAL\n", string(content))
	require.Empty(t, r.logged(t), "no afterFileEdit fired")
}

// TestAnExit2InPreToolUseRefusesAReadWithAnErrorResult: recorded, a preToolUse
// hook exiting 2 on a Read refuses it: the file is not read, no postToolUse
// fires for the call, the failure hook reports permission_denied with the
// hook's stderr after "Hook blocked with message:", and the read's result to the
// agent is an error carrying that message and the note not to look for
// workarounds.
// sr:proves pretooluse-refusal/cursor
func TestAnExit2InPreToolUseRefusesAReadWithAnErrorResult(t *testing.T) {
	got, want := replay(t, "pretool-refusal-file-tools")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		var reads []string
		for _, h := range o.hooks {
			if h["tool_name"] != "Read" || h["tool_input"].(map[string]any)["file_path"] != "<RUN>/secret.txt" {
				continue
			}
			reads = append(reads, h["hook_event_name"].(string))
			if h["hook_event_name"] == "postToolUseFailure" {
				require.Equal(t, "Hook blocked with message: READ-BLOCK-MSG", h["error_message"], name)
				require.Equal(t, "permission_denied", h["failure_type"], name)
			}
		}
		require.Equal(t, []string{"preToolUse", "postToolUseFailure"}, reads, name+": the refused read has no postToolUse")
		require.Contains(t, o.frames, "tool_call/completed/readToolCall/error", name)
		require.NotContains(t, o.frames, "tool_call/completed/readToolCall/success", name)
	}

	block := "#!/bin/sh\ncat >/dev/null\necho READ-BLOCK-MSG >&2\nexit 2\n"
	r := runTools(t, `{"version":1,"hooks":{"preToolUse":[{"command":".cursor/hooks/block.sh","matcher":"Read"}]}}`,
		map[string]string{"block.sh": block}, map[string]string{"secret.txt": "SECRET\n"},
		map[string]any{"name": "Read", "input": map[string]any{"file_path": "secret.txt"}})
	res := r.completed("readToolCall", 0)
	require.NotContains(t, res, "success")
	e, _ := res["error"].(map[string]any)
	require.Equal(t, "Hook blocked with message: READ-BLOCK-MSG\n\nAgent note: Do not suggest workarounds to the blocked tool.", e["errorMessage"])
	for _, f := range r.frames { // the file's content never reaches the agent
		require.NotContains(t, jsonString(f), "SECRET")
	}
}
