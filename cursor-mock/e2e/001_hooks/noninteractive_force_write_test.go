package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestForceAppliesAFileWriteAndRunsTheCommand: recorded (runs/noninteractive-force-write,
// cursor-agent -p --force --trust), the same prompt as the no-force run writes
// note.txt, runs echo FINE and reads the file back: the write is applied (the
// sessionEnd hook finds note.txt holding hi), the command runs and its result
// is a success whose after-hook is given its output, and the read succeeds.
// The mock's replay under --force gives the same frames and the same file.
// sr:proves noninteractive-run/cursor
func TestForceAppliesAFileWriteAndRunsTheCommand(t *testing.T) {
	got, want := replay(t, "noninteractive-force-write")
	conforms(t, got, want)

	require.Equal(t, []string{
		"tool_call/started/editToolCall/", "tool_call/completed/editToolCall/success",
		"tool_call/started/shellToolCall/", "tool_call/completed/shellToolCall/success",
		"tool_call/started/readToolCall/", "tool_call/completed/readToolCall/success",
		"result/success",
	}, want.frames, "recorded")
	require.Equal(t, want.frames, got.frames)

	for _, m := range readJSONL(t, filepath.Join(newestSample(t, "noninteractive-force-write"), "payloads.jsonl")) {
		if r, ok := m["hook_result"].(map[string]any); ok && r["event"] == "files" {
			require.Equal(t, true, r["note_exists"])
			require.Equal(t, "hi", r["note"])
		}
	}
	b, err := os.ReadFile(filepath.Join(got.ws, "note.txt"))
	require.NoError(t, err)
	require.Equal(t, "hi\n", string(b))

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for _, h := range o.hooks {
			if h["hook_event_name"] == "afterShellExecution" {
				require.Equal(t, "FINE\n", h["output"], name)
			}
		}
	}
}

// toolCallBodies projects each tool_call frame of a stream to what the
// recording pins of it: the call id, the tool, the started frame's args and the
// completed frame's success fields (the paths as <RUN>), keyed by subtype and
// call.
func toolCallBodies(t *testing.T, frames []map[string]any, ws string) map[string]any {
	t.Helper()
	fields := map[string][]string{
		"editToolCall":  {"path", "linesAdded", "linesRemoved", "afterFullFileContent"},
		"shellToolCall": {"command", "stdout", "exitCode"},
		"readToolCall":  {"path", "content", "totalLines", "fileSize"},
	}
	out := map[string]any{}
	for _, f := range frames {
		if f["type"] != "tool_call" {
			continue
		}
		for kind, v := range f["tool_call"].(map[string]any) {
			body, ok := v.(map[string]any)
			if !ok || fields[kind] == nil {
				continue
			}
			args, _ := body["args"].(map[string]any)
			proj := map[string]any{"call_id": f["call_id"], "args": map[string]any{}}
			for _, k := range []string{"path", "command", "streamContent"} {
				if a, ok := args[k]; ok {
					proj["args"].(map[string]any)[k] = a
				}
			}
			if res, _ := body["result"].(map[string]any); res != nil {
				succ, _ := res["success"].(map[string]any)
				require.NotNil(t, succ, "a completed %s without success: %v", kind, res)
				for _, k := range fields[kind] {
					require.Contains(t, succ, k, kind)
					proj[k] = succ[k]
				}
			}
			b, err := json.Marshal(proj)
			require.NoError(t, err)
			var norm any
			require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(string(b), ws, "<RUN>")), &norm))
			out[fmt.Sprint(f["subtype"], "/", kind)] = norm
		}
	}
	return out
}

// TestToolCallFramesCarryTheRecordedBodies: the tool_call frames of the
// recorded run (runs/noninteractive-force-write) carry the call id and the
// started args (the edit's path and streamContent, the command, the read's
// path) and, completed, the result's success fields: for the edit linesAdded,
// linesRemoved, afterFullFileContent and path, for the command its stdout and
// exit code, for the read content, totalLines, fileSize and path. The mock's
// frames for the same calls carry the same values.
// sr:proves noninteractive-run/cursor
func TestToolCallFramesCarryTheRecordedBodies(t *testing.T) {
	got, _ := replay(t, "noninteractive-force-write")
	recorded := readJSONL(t, filepath.Join(newestSample(t, "noninteractive-force-write"), "stream.jsonl"))
	want := toolCallBodies(t, recorded, "\x00")
	have := toolCallBodies(t, readJSONLText(t, got.stdout), got.ws)
	require.Len(t, want, 6)
	require.Equal(t, want["completed/editToolCall"].(map[string]any)["afterFullFileContent"], "hi\n")
	require.Equal(t, want["completed/shellToolCall"].(map[string]any)["stdout"], "FINE\n")
	require.Equal(t, want["completed/readToolCall"].(map[string]any)["totalLines"], float64(2))
	require.Equal(t, want, have)
}
