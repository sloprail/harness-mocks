package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/internal/toolspec"
)

var runs = filepath.Join("..", "..", "snapshots", "runs")

// recordedCalls are the tool calls the model made in every recorded run: the
// tool_use blocks of the assistant records of each transcript, sub-agents' too.
func recordedCalls(t *testing.T) []toolspec.Recorded { return recordedCallsOf(t, "*") }

// recordedCallsOf are those of the named run.
func recordedCallsOf(t *testing.T, run string) []toolspec.Recorded {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(runs, run, "samples", "*", "transcript", "*.jsonl"))
	require.NoError(t, err)
	subs, err := filepath.Glob(filepath.Join(runs, run, "samples", "*", "transcript", "*", "subagents", "*.jsonl"))
	require.NoError(t, err)
	var calls []toolspec.Recorded
	for _, f := range append(files, subs...) {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, line := range strings.Split(string(b), "\n") {
			var rec struct {
				Type    string `json:"type"`
				Message struct {
					Content []struct {
						Type  string         `json:"type"`
						Name  string         `json:"name"`
						Input map[string]any `json:"input"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(line), &rec) != nil || rec.Type != "assistant" {
				continue
			}
			for _, b := range rec.Message.Content {
				if b.Type == "tool_use" {
					calls = append(calls, toolspec.Recorded{Tool: b.Name, Input: b.Input})
				}
			}
		}
	}
	require.NotEmpty(t, calls)
	return calls
}

// Every tool and parameter the schema declares is one a recorded run shows the
// model use, and every call a recording shows of a declared tool is one the
// mock can play (adr/tool-calls-validated).
func TestTheSchemaIsGroundedInTheRecordings(t *testing.T) {
	require.Empty(t, Schema().Ungrounded(recordedCalls(t)))
}

// Each mistake the schema says Claude Code answers itself is one a recorded run
// shows it answering: the run holds a call of that tool without the required
// parameter, and its transcript the InputValidationError naming it. The exact
// answer the mock gives is pinned by TestT017_56_AgentDispatchWithoutRequiredInputIsRefusedBeforeAnyHook
// and TestT017_29c_InvalidInputFiresNoHook (e2e/017_session_lifecycle).
func TestTheAnsweredKindsAreInTheirRecordings(t *testing.T) {
	missing := map[string]string{"Agent": "prompt", "Task": "prompt", "Read": "file_path"} // the parameter each answered tool's recorded call lacks
	for _, tool := range Schema().Tools {
		for kind, run := range tool.Answers {
			param := missing[tool.Name]
			require.NotEmpty(t, param, "%s: say which required parameter its recorded call lacks", tool.Name)
			recorded := tool.Recorded
			if recorded == "" {
				recorded = tool.Name
			}
			var lacking bool
			for _, c := range recordedCallsOf(t, run) {
				_, has := c.Input[param]
				lacking = lacking || c.Tool == recorded && !has
			}
			require.True(t, lacking, "%s (%s): %s holds no call of %s without %s", tool.Name, kind, run, recorded, param)
			files, _ := filepath.Glob(filepath.Join(runs, run, "samples", "*", "transcript", "*.jsonl"))
			require.NotEmpty(t, files, "%s: %s has no transcript", tool.Name, run)
			b, err := os.ReadFile(files[0])
			require.NoError(t, err)
			require.Contains(t, string(b), "The required parameter `"+param+"` is missing", "%s (%s): %s does not show Claude Code's answer", tool.Name, kind, run)
		}
	}
}

// A script's call of an unknown tool, an unknown parameter or a wrong type is
// refused before it is played.
func TestTheSchemaRefusesWhatTheMockDoesNotImplement(t *testing.T) {
	for in, call := range map[string][2]string{
		"unknown tool":      {"Teleport", `{}`},
		"unknown parameter": {"Bash", `{"command":"ls","tty":true}`},
		"wrong type":        {"Bash", `{"command":["ls"]}`},
		"value":             {"Agent", `{"description":"d","prompt":"p","isolation":"container"}`},
	} {
		_, err := Schema().Check(call[0], json.RawMessage(call[1]))
		require.Error(t, err, in)
	}
}
