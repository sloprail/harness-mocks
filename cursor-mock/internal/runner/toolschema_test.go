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
func recordedCalls(t *testing.T) []toolspec.Recorded {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(runs, "*", "samples", "*", "transcript", "*", "*.jsonl"))
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
// read), which the mock does not implement. A script that asks for it is refused,
// and the run that shows it is not replayable until the mock reads a range.
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
// it answering, with the text the mock answers with.
func TestTheAnsweredKindsAreInTheirRecordings(t *testing.T) {
	for _, tool := range Schema().Tools {
		for kind, run := range tool.Answers {
			files, _ := filepath.Glob(filepath.Join(runs, run, "samples", "*", "stream.jsonl"))
			require.NotEmpty(t, files, "%s: %s has no stream", tool.Name, run)
			b, err := os.ReadFile(files[0])
			require.NoError(t, err)
			require.Contains(t, string(b), "Invalid arguments:", "%s (%s): %s does not show Cursor's answer", tool.Name, kind, run)
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
