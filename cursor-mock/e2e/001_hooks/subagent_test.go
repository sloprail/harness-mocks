package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/subagent-transcripts: the agent spawns one sub-agent
// with its Task tool.

// taskThenDone makes one Task call and then ends.
const taskThenDone = `#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_0","name":"Task","input":{"description":"Reply with PONG","prompt":"Reply with exactly the word PONG and nothing else.","subagent_type":"generalPurpose","script":"SUBSCRIPT"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}'
`

// pongScript plays the sub-agent: it answers PONG.
const pongScript = `#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"PONG"}]}}' '{"type":"result","subtype":"success","result":"PONG"}'
`

// transcripts are the conversation transcripts under the agent-transcripts
// directory of a home or of a recorded sample: by conversation id, the file's
// records.
func transcripts(t *testing.T, root string) map[string][]map[string]any {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	require.NoError(t, err)
	out := map[string][]map[string]any{}
	for _, f := range files {
		assert.Equal(t, filepath.Base(filepath.Dir(f))+".jsonl", filepath.Base(f), "a directory and a file named by the conversation")
		out[filepath.Base(filepath.Dir(f))] = readJSONL(t, f)
	}
	return out
}

// A sub-agent's records go to a transcript of its own: a conversation of its
// own among the session's under agent-transcripts, not part of the session's
// file. The session's holds the Task call and the stream's completed frame
// names the sub-agent by the id of that transcript; the sub-agent's holds the
// task it was given. No file beside either says its type, the call that
// spawned it or its depth (runs/subagent-transcripts).
// sr:proves subagent-transcripts/cursor
func TestSubAgentRecordsGoToATranscriptOfItsOwn(t *testing.T) {
	sample := newestSample(t, "subagent-transcripts")
	want := transcripts(t, filepath.Join(sample, "transcript"))
	require.Len(t, want, 2)

	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	script := filepath.Join(scratch, "scenario.sh")
	sub := filepath.Join(scratch, "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(pongScript), 0o755))
	require.NoError(t, os.WriteFile(script, []byte(strings.ReplaceAll(taskThenDone, "SUBSCRIPT", sub)), 0o755))
	cmd := exec.Command(binary, "-p", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	roots, err := filepath.Glob(filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts"))
	require.NoError(t, err)
	require.Len(t, roots, 1)
	got := transcripts(t, roots[0])
	require.Len(t, got, 2, "the session's transcript and the sub-agent's")

	// the stream says which: the completed Task frame names the sub-agent
	agentID := func(frames []map[string]any) string {
		for _, f := range frames {
			call, _ := f["tool_call"].(map[string]any)
			tc, _ := call["taskToolCall"].(map[string]any)
			if res, ok := tc["result"].(map[string]any); ok && f["subtype"] == "completed" {
				return res["success"].(map[string]any)["agentId"].(string)
			}
		}
		return ""
	}
	wantChild := agentID(readJSONL(t, filepath.Join(sample, "stream.jsonl")))
	gotChild := agentID(jsonLinesOf(string(out)))
	require.NotEmpty(t, wantChild)
	require.NotEmpty(t, gotChild)
	require.Contains(t, want, wantChild, "recording: the transcript is named by the id the frame gives")
	require.Contains(t, got, gotChild)

	holds := func(recs []map[string]any, text string) bool { return strings.Contains(jsonString(recs), text) }
	// the task is what a conversation is asked: its user records
	asked := func(recs []map[string]any) (out []map[string]any) {
		for _, r := range recs {
			if r["role"] == "user" {
				out = append(out, r)
			}
		}
		return
	}
	for name, p := range map[string]struct {
		all   map[string][]map[string]any
		child string
	}{"recording": {want, wantChild}, "mock": {got, gotChild}} {
		for id, recs := range p.all {
			if id == p.child {
				assert.True(t, holds(asked(recs), "Reply with exactly the word PONG"), "%s: the sub-agent's file holds its task", name)
				assert.False(t, holds(recs, `"name":"Task"`), "%s: and not the session's call", name)
			} else {
				assert.True(t, holds(recs, `"name":"Task"`), "%s: the session's holds the call", name)
				assert.False(t, holds(asked(recs), "Reply with exactly the word PONG"), "%s: and not the sub-agent's task", name)
			}
		}
	}
	// nothing else is written beside them: no sidecar naming type, call or depth
	for _, root := range []string{filepath.Join(sample, "transcript"), roots[0]} {
		var files []string
		require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				files = append(files, p)
			}
			return nil
		}))
		assert.Len(t, files, 2, root)
	}
}

func jsonLinesOf(text string) (out []map[string]any) {
	for _, l := range strings.Split(text, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m != nil {
			out = append(out, m)
		}
	}
	return out
}
