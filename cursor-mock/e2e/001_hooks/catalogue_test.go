package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestACatalogueSearchIsAnsweredOnlyWhereARecordingShowsItFindingNothing:
// recorded (runs/schedule-wakeup-ask), a search of the harness's tool catalogue
// for a pattern that matches none of its tools answers an empty list of matches;
// a search that matches (runs/stop-hook-payload, the pattern ".") answers the
// catalogue, which the mock has none of. The mock answers the first and refuses
// every other search as not modeled, rather than say nothing was found.
// sr:proves noninteractive-run/cursor
func TestACatalogueSearchIsAnsweredOnlyWhereARecordingShowsItFindingNothing(t *testing.T) {
	recorded := taskLikeContent(t, "schedule-wakeup-ask")
	require.Contains(t, recorded, `"matches": []`, "recorded: nothing found")

	run := func(pattern string, subAgent bool) (string, error) {
		scratch := t.TempDir()
		call := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"GetDynamicTools","input":{"pattern":"` + pattern + `"}}]}}`
		leaf := filepath.Join(scratch, "leaf.sh")
		require.NoError(t, os.WriteFile(leaf, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then printf '%s\n' '`+call+`'; else printf '%s\n' '{"type":"result","subtype":"success","result":"LEAF"}'; fi
`), 0o755))
		first := call
		if subAgent { // the search is made by a sub-agent, whose frames are not printed
			first = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Task","input":{"description":"d","prompt":"p","subagent_type":"generalPurpose","script":"` + leaf + `"}}]}}`
		}
		script := filepath.Join(scratch, "s.sh")
		require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then printf '%s\n' '`+first+`'; else printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}'; fi
`), 0o755))
		cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
		cmd.Dir, cmd.Env = t.TempDir(), []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run("subscribe_timer|cursor-subscriptions", false)
	require.NoError(t, err, out)
	require.Contains(t, out, `\"matches\": []`)
	refused, err := run(".", false)
	require.Error(t, err, "a refusal of something not modeled fails the run")
	require.Contains(t, refused, "is not modeled")
	require.NotContains(t, refused, `\"matches\"`)
	// and a sub-agent's refusal, whose frames are never printed, fails it just the same
	hidden, err := run(".", true)
	require.Error(t, err, "a sub-agent's refusal must not pass unseen")
	require.Contains(t, hidden, "is not modeled")
}

// taskLikeContent is the content of the first getMcpToolsToolCall result a run's
// stream holds.
func taskLikeContent(t *testing.T, run string) string {
	t.Helper()
	for _, f := range readJSONL(t, filepath.Join(newestSample(t, run), "stream.jsonl")) {
		tc, _ := f["tool_call"].(map[string]any)
		body, _ := tc["getMcpToolsToolCall"].(map[string]any)
		if r, _ := body["result"].(map[string]any); r != nil {
			if s, _ := r["success"].(map[string]any); s != nil {
				return s["content"].(string)
			}
		}
	}
	t.Fatalf("%s: no catalogue search answered", run)
	return ""
}
