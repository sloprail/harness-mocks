package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// refusedResults are the results of the completed tool-call frames that report
// a refusal (a rejection, or an error), in order, from a stream of frames (one
// JSON object per line).
func refusedResults(t *testing.T, frames []map[string]any) (out []map[string]any) {
	t.Helper()
	for _, f := range frames {
		if f["type"] != "tool_call" || f["subtype"] != "completed" {
			continue
		}
		for k, v := range f["tool_call"].(map[string]any) {
			if !strings.HasSuffix(k, "ToolCall") {
				continue
			}
			res, _ := v.(map[string]any)["result"].(map[string]any)
			if _, ok := res["rejected"]; ok {
				out = append(out, res)
			}
			if _, ok := res["error"]; ok {
				out = append(out, res)
			}
		}
	}
	return out
}

func framesOf(t *testing.T, stdout string) (out []map[string]any) {
	t.Helper()
	for _, l := range strings.Split(stdout, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			out = append(out, f)
		}
	}
	return out
}

// TestARefusedCommandsResultToTheAgentIsTheRecordedRejection: recorded
// (runs/pretool-refusal), the result frame of a command a hook refused is a
// rejection of that command, run from no working directory of its own, with
// the reason the agent is told and isReadonly false: the hook's message and the
// note not to look for workarounds when preToolUse refused it, and, when
// beforeShellExecution did, the message after "Command execution was blocked
// by a hook:", the pointer to the hook settings and the note. The mock's frames
// carry the same objects, field for field.
// sr:proves pretooluse-refusal/cursor
func TestARefusedCommandsResultToTheAgentIsTheRecordedRejection(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	recorded := refusedResults(t, readJSONL(t, filepath.Join(newestSample(t, "pretool-refusal"), "stream.jsonl")))
	mock := refusedResults(t, framesOf(t, got.stdout))
	require.Len(t, recorded, 5, "the five refused commands")
	require.Equal(t, recorded, mock)

	const note = "\n\nAgent note: Do not suggest workarounds to the blocked tool."
	const settings = "\n\nTo view or modify configured hooks, go to Cursor Settings > Hooks."
	reasons := map[string]string{}
	for _, r := range mock {
		rej := r["rejected"].(map[string]any)
		require.Equal(t, "", rej["workingDirectory"])
		require.Equal(t, false, rej["isReadonly"])
		reasons[rej["command"].(string)] = rej["reason"].(string)
	}
	require.Equal(t, map[string]string{
		"echo DENYME":     "USER-DENY-MSG" + note,
		"echo EXIT2PRE":   "Hook blocked with message: PRE-BLOCK-MSG" + note,
		"echo DENYWINS":   "USER-DENY-MSG" + note,
		"echo EXIT2SHELL": "Command execution was blocked by a hook: Hook blocked with message: SHELL-BLOCK-MSG" + settings + note,
		"echo DENYSHELL":  "Command execution was blocked by a hook: SHELL-USER-DENY-MSG" + settings + note,
	}, reasons)
}
