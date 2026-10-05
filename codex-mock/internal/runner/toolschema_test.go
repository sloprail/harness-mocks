package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/codex-mock/internal/replay"
	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/toolspec"
)

var runs = filepath.Join("..", "..", "snapshots", "runs")

// recordedCalls are the tool calls the model made in every recorded run, read
// by the replay adapter into the unified form and put back in Codex's names
// (exec_command's cmd).
func recordedCalls(t *testing.T) []toolspec.Recorded {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(runs, "*"))
	require.NoError(t, err)
	var calls []toolspec.Recorded
	var walk func(a core.Agent)
	walk = func(a core.Agent) {
		for _, c := range a.Calls {
			in := map[string]any{}
			for k, v := range c.Input {
				in[k] = v
			}
			switch c.Tool {
			case core.ToolShell:
				in["cmd"] = in["command"]
				delete(in, "command")
				calls = append(calls, toolspec.Recorded{Tool: "exec_command", Input: in})
			case core.ToolSpawn:
				spawn := map[string]any{}
				if in["message"] != nil {
					spawn["message"] = in["message"]
				}
				calls = append(calls, toolspec.Recorded{Tool: agentTool, Input: spawn})
			}
			if c.Sub != nil {
				walk(*c.Sub)
			}
		}
	}
	for _, d := range dirs {
		rec, err := replay.Adapter{}.Load(d)
		if err != nil { // a run the adapter cannot read has no calls here
			continue
		}
		walk(rec.Agent)
	}
	require.NotEmpty(t, calls)
	return calls
}

// spawnCalls are the spawn_agent calls a rollout holds as function calls (the
// others are in the model's JavaScript, which the replay adapter reads).
func spawnCalls(t *testing.T, rollout []byte) []toolspec.Recorded {
	var out []toolspec.Recorded
	for _, line := range strings.Split(string(rollout), "\n") {
		var rec struct {
			Payload struct {
				Type, Name, Arguments string
			} `json:"payload"`
		}
		var in map[string]any
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Payload.Type == "function_call" && rec.Payload.Name == agentTool &&
			json.Unmarshal([]byte(rec.Payload.Arguments), &in) == nil {
			out = append(out, toolspec.Recorded{Tool: agentTool, Input: in})
		}
	}
	return out
}

// Every tool and parameter the schema declares is one a recorded run shows the
// model use, and every call a recording shows of a declared tool is one the mock
// can play or Codex itself answers (adr/tool-calls-validated). apply_patch is
// not one the replay adapter reads, so a rollout that calls it is looked for
// directly.
func TestTheSchemaIsGroundedInTheRecordings(t *testing.T) {
	var problems []string
	patched := false
	rollouts, _ := filepath.Glob(filepath.Join(runs, "*", "samples", "*", "transcript", "rollout-*.jsonl"))
	calls := recordedCalls(t)
	for _, f := range rollouts {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		patched = patched || strings.Contains(string(b), "tools.apply_patch(")
		calls = append(calls, spawnCalls(t, b)...)
	}
	for _, p := range Schema().Ungrounded(calls) {
		if patched && strings.Contains(p, "tool "+patchTool+" is declared") {
			continue
		}
		problems = append(problems, p)
	}
	require.Empty(t, problems)
}

// A script's call of an unknown tool, an unknown parameter or a wrong type is
// refused before it is played.
func TestTheSchemaRefusesWhatTheMockDoesNotImplement(t *testing.T) {
	for in, call := range map[string][2]string{
		"unknown tool":      {"exec_command", `{"cmd":"ls"}`},
		"unknown parameter": {"Bash", `{"command":"ls","sandbox_permissions":"x"}`},
		"wrong type":        {"Bash", `{"command":7}`},
		"missing":           {"Bash", `{}`},
	} {
		_, err := Schema().Check(call[0], json.RawMessage(call[1]))
		require.Error(t, err, in)
	}
	answered, err := Schema().Check(agentTool, json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Len(t, answered, 1, "Codex answers a spawn_agent without a message itself")
}

// The answered kind is in its recording, with the text the mock answers with.
func TestTheAnsweredKindsAreInTheirRecordings(t *testing.T) {
	for _, tool := range Schema().Tools {
		for kind, run := range tool.Answers {
			files, _ := filepath.Glob(filepath.Join(runs, run, "samples", "*", "transcript", "rollout-*.jsonl"))
			require.NotEmpty(t, files, "%s: %s has no rollout", tool.Name, run)
			b, err := os.ReadFile(files[0])
			require.NoError(t, err)
			require.Contains(t, string(b), spawnRefusal, "%s (%s): %s does not show Codex's answer", tool.Name, kind, run)
		}
	}
}
