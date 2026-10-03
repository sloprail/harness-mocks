package e2e

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedRaw are the payloads of a run's newest sample as the real hooks read
// them (payloads.jsonl keeps the fields the normalized events drop).
func recordedRaw(t *testing.T, run string) []map[string]any {
	t.Helper()
	samples, err := filepath.Glob(filepath.Join("..", "..", "snapshots", "runs", run, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	sort.Strings(samples)
	var out []map[string]any
	for _, m := range readJSONL(t, filepath.Join(samples[len(samples)-1], "payloads.jsonl")) {
		if m["hook_event_name"] != nil && m["hook_event_name"] != "afterAgentThought" && m["hook_event_name"] != "BackgroundTick" {
			out = append(out, m)
		}
	}
	return out
}

// withCwd is, for each (event, tool) a run's payloads show, whether they carry
// a cwd field.
func withCwd(payloads []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, p := range payloads {
		tool, _ := p["tool_name"].(string)
		_, has := p["cwd"]
		out[p["hook_event_name"].(string)+"/"+tool] = has
	}
	return out
}

// TestTheWorkingDirectoryIsNamedByWorkspaceRootsAndOnlyAShellEventHasACwd:
// recorded, every payload names the project as workspace_roots; a cwd field,
// empty when the command ran from the project root, is carried by the events
// of the Shell tool (beforeShellExecution, and preToolUse, postToolUse and
// postToolUseFailure of a Shell call) and by no other event, a Read, Write or
// Task call's included; the transcript path is null at sessionStart and
// before the conversation's transcript exists. The mock's payloads match.
// sr:proves hook-common-payload/cursor
func TestTheWorkingDirectoryIsNamedByWorkspaceRootsAndOnlyAShellEventHasACwd(t *testing.T) {
	for _, run := range []string{"file-tools", "tool-failure", "pretool-refusal"} {
		t.Run(run, func(t *testing.T) {
			got, want := replay(t, run)
			conforms(t, got, want)
			recorded := recordedRaw(t, run)
			require.NotEmpty(t, recorded)

			for _, p := range recorded {
				assert.NotEmpty(t, p["workspace_roots"], p["hook_event_name"])
				tool, _ := p["tool_name"].(string)
				if _, has := p["cwd"]; has {
					assert.Equal(t, "", p["cwd"], "recorded from the project root")
					assert.True(t, tool == "Shell" || p["hook_event_name"] == "beforeShellExecution", "a cwd on %v/%s", p["hook_event_name"], tool)
				}
				if p["hook_event_name"] == "sessionStart" {
					assert.Nil(t, p["transcript_path"])
				}
			}

			gotCwd := withCwd(got.raw)
			for k, has := range withCwd(recorded) {
				if mockHas, seen := gotCwd[k]; seen {
					assert.Equal(t, has, mockHas, "cwd on %s", k)
				}
			}
			for _, p := range got.raw {
				assert.Equal(t, []any{got.ws}, p["workspace_roots"])
				if _, has := p["cwd"]; has {
					assert.Equal(t, "", p["cwd"], "the mock reports the project root as the recorded empty cwd")
				}
			}
		})
	}
}
