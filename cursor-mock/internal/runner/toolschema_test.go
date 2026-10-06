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
	files, err := filepath.Glob(filepath.Join(runs, run, "samples", "*", "transcript", "*", "*.jsonl"))
	require.NoError(t, err)
	var calls []toolspec.Recorded
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, line := range strings.Split(string(b), "\n") {
			var rec struct {
				Role    string `json:"role"`
				Message struct {
					Content []struct {
						Type  string         `json:"type"`
						Name  string         `json:"name"`
						Input map[string]any `json:"input"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(line), &rec) != nil || rec.Role != "assistant" {
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
// mock can play or Cursor itself answers (adr/tool-calls-validated).
//
// One recorded call the mock cannot play yet: a Read with a limit (the lines to
// read) in runs/compaction-transcript-continuity, which has only a started frame,
// no completed one: what a limited read returns is not recorded, so the mock
// refuses a script that asks for it rather than invent it.
func TestTheSchemaIsGroundedInTheRecordings(t *testing.T) {
	var problems []string
	for _, p := range Schema().Ungrounded(recordedCalls(t)) {
		if !strings.Contains(p, `Read: unknown parameter "limit"`) {
			problems = append(problems, p)
		}
	}
	require.Empty(t, problems)
}

// Each mistake the schema says Cursor answers itself is one a recorded run shows
// it answering: the run holds a call of that tool without the required parameter
// the answer names, and the stream's completed frame carries the answer's text.
// The exact text and frame the mock answers with are pinned by
// TestTaskWithoutPromptIsRefusedBeforeAnyHook (e2e/001_hooks).
func TestTheAnsweredKindsAreInTheirRecordings(t *testing.T) {
	missing := map[string]string{"Task": "prompt", "Agent": "prompt"} // the parameter each answered tool's recorded call lacks
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
			streams, _ := filepath.Glob(filepath.Join(runs, run, "samples", "*", "stream.jsonl"))
			require.NotEmpty(t, streams, "%s: %s has no stream", tool.Name, run)
			b, err := os.ReadFile(streams[0])
			require.NoError(t, err)
			require.Contains(t, string(b), "Invalid arguments:\\n"+param+": Required", "%s (%s): %s does not show Cursor's answer", tool.Name, kind, run)
		}
	}
}

// A script's call of an unknown tool, an unknown parameter or a wrong type is
// refused before it is played.
func TestTheSchemaRefusesWhatTheMockDoesNotImplement(t *testing.T) {
	for in, call := range map[string][2]string{
		"unknown tool":      {"Teleport", `{}`},
		"unknown parameter": {"Shell", `{"command":"ls","run_in_background":true}`},
		"wrong type":        {"Read", `{"file_path":3}`},
		"value":             {"Shell", `{"command":"ls","block_until_ms":7}`},
	} {
		_, err := Schema().Check(call[0], json.RawMessage(call[1]))
		require.Error(t, err, in)
	}
}
