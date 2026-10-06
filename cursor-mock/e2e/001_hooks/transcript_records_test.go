package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordKinds names what a transcript's records are: the role and the kinds of
// content blocks of each, and the turn_ended line that closes it.
func recordKinds(records []map[string]any) (out []string) {
	for _, r := range records {
		if r["type"] == "turn_ended" {
			out = append(out, "turn_ended/"+r["status"].(string))
			continue
		}
		kind, _ := r["role"].(string)
		if msg, _ := r["message"].(map[string]any); msg != nil {
			for _, b := range msg["content"].([]any) {
				kind += "/" + b.(map[string]any)["type"].(string)
			}
		}
		out = append(out, kind)
	}
	return out
}

// TestTheSessionFileHoldsTheUserRecordThenOneAssistantRecordPerStepThenTurnEnded:
// recorded (runs/tool-failure), the transcript file is the user's record, then
// an assistant record for each step, its text and tool_use blocks, and ends
// with a turn_ended line whose status is success; the mock's file is the same
// sequence for the same calls.
// sr:proves session-transcript-file/cursor
func TestTheSessionFileHoldsTheUserRecordThenOneAssistantRecordPerStepThenTurnEnded(t *testing.T) {
	got, _ := replay(t, "tool-failure")
	recorded, err := filepath.Glob(filepath.Join(newestSample(t, "tool-failure"), "transcript", "*", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, recorded, 1)
	mock, err := filepath.Glob(filepath.Join(got.home, ".cursor", "projects", "*", "agent-transcripts", "*", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, mock, 1)

	want := recordKinds(readJSONL(t, recorded[0]))
	require.Equal(t, "user/text", want[0], "recorded: the user's record first")
	require.Equal(t, "turn_ended/success", want[len(want)-1], "recorded: the turn_ended line last")
	assert.Contains(t, want, "assistant/text/tool_use", "recorded: a step's text and its call in one record")
	assert.Contains(t, want, "assistant/tool_use")
	assert.Equal(t, "assistant/text", want[len(want)-2], "recorded: the last step's words are a record of their own")

	// the replay's script says nothing between its calls: compared without the text
	withoutText := func(kinds []string) (out []string) {
		for _, k := range kinds {
			k = strings.ReplaceAll(k, "/text", "")
			if k != "assistant" {
				out = append(out, strings.Replace(k, "user", "user/text", 1))
			}
		}
		return out
	}
	assert.Equal(t, withoutText(want), withoutText(recordKinds(readJSONL(t, mock[0]))), "the mock's file has the same records for the same calls")

	// and a script that says something before a call, and after the last: the mock's
	// records are the recorded kinds
	scratch, ws, home := t.TempDir(), t.TempDir(), t.TempDir()
	script := filepath.Join(scratch, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"STARTING"}]}}' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"Bash","input":{"command":"echo hi"}}]}}'
else
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"FINISHED"}]}}' '{"type":"result","subtype":"success","result":"FINISHED"}'
fi
`), 0o755))
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	files, err := filepath.Glob(filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts", "*", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, []string{"user/text", "assistant/text/tool_use", "assistant/text", "turn_ended/success"}, recordKinds(readJSONL(t, files[0])))
}
