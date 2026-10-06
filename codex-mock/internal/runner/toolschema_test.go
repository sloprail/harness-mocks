package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/codex-mock/internal/replay"
	"github.com/sloprail/harness-mocks/internal/toolspec"
)

var runs = filepath.Join("..", "..", "snapshots", "runs")

// recordedCalls are the tool calls the model made in every recorded run, sub-agents' too: read out
// of the JS each rollout's calls are made from.
func recordedCalls(t *testing.T) []toolspec.Recorded {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(runs, "*", "samples", "*", "transcript", "*.jsonl"))
	require.NoError(t, err)
	var calls []toolspec.Recorded
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		var records []map[string]any
		for _, line := range strings.Split(string(b), "\n") {
			var rec map[string]any
			if json.Unmarshal([]byte(line), &rec) == nil && rec != nil {
				records = append(records, rec)
			}
		}
		for _, c := range replay.RolloutCalls(records) {
			calls = append(calls, toolspec.Recorded{Tool: c.Tool, Input: c.Input})
		}
	}
	require.NotEmpty(t, calls)
	return calls
}

// Every tool and parameter the schema declares is one a recorded run shows the model use, and every
// call a recording shows of a declared tool is one the mock can play or Codex itself answers
// (adr/tool-calls-validated).
func TestTheSchemaIsGroundedInTheRecordings(t *testing.T) {
	require.Empty(t, Schema().Ungrounded(recordedCalls(t)))
}

// Each mistake the schema says Codex answers itself is one a recorded run shows it answering.
func TestTheAnsweredKindsAreInTheirRecordings(t *testing.T) {
	for _, tool := range Schema().Tools {
		for kind, run := range tool.Answers {
			files, _ := filepath.Glob(filepath.Join(runs, run, "samples", "*", "transcript", "*.jsonl"))
			require.NotEmpty(t, files, "%s (%s): %s has no rollout", tool.Name, kind, run)
		}
	}
}

// A script's call of an unknown tool, an unknown parameter, a wrong type or an option value the mock
// does not implement is refused before it is played.
func TestTheSchemaRefusesWhatTheMockDoesNotImplement(t *testing.T) {
	for in, call := range map[string][2]string{
		"unknown tool":      {"Teleport", `{}`},
		"unknown parameter": {"Bash", `{"command":"ls","sandbox":"none"}`},
		"wrong type":        {"Bash", `{"command":3}`},
		"value":             {"Bash", `{"command":"ls","shell":"bash"}`},
		"missing":           {"wait_agent", `{}`},
	} {
		_, err := Schema().Check(call[0], json.RawMessage(call[1]))
		require.Error(t, err, in)
	}
}
