package e2e

import (
	"os"
	"path/filepath"
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
