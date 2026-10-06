package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-matchers-thought (cursor-agent with a model that
// reports its thinking): workspaceOpen, afterAgentThought and sessionEnd hooks
// each configured with a matcher naming the event, a matcher that matches
// nothing and no matcher, around a Read call.

// TestAMatcherIsTestedAgainstAgentThoughtButNotAgainstWorkspaceOpenOrSessionEnd:
// recorded, an afterAgentThought hook's matcher is tested against AgentThought:
// the hook matched on it and the one with no matcher run for each thought, one
// matched on nothing does not; workspaceOpen and sessionEnd have no subject, so
// every hook of them runs whatever its matcher. The mock does the same.
// sr:proves hook-matcher-filter/cursor
func TestAMatcherIsTestedAgainstAgentThoughtButNotAgainstWorkspaceOpenOrSessionEnd(t *testing.T) {
	setup, want, _, _ := recording(t, "hook-matchers-thought")
	require.Contains(t, want.results, "ran:thought-AgentThought:afterAgentThought:::<nil>")
	require.NotContains(t, want.results, "ran:thought-other:afterAgentThought:::<nil>")
	require.Contains(t, want.results, "ran:open-other:workspaceOpen:::<nil>", "recorded: a workspaceOpen hook runs whatever its matcher")
	require.Contains(t, want.results, "ran:end-other:sessionEnd:::<nil>", "recorded: a sessionEnd hook runs whatever its matcher")

	ws, home, scratch := t.TempDir(), t.TempDir(), t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "ran.sh"), filepath.Join(ws, ".cursor", "hooks", "ran.sh"), 0o755)
	require.NoError(t, os.WriteFile(filepath.Join(ws, "note.txt"), []byte("hi\n"), 0o644))
	script := filepath.Join(scratch, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Running the command."},{"type":"tool_use","id":"c1","name":"Read","input":{"file_path":"note.txt"}}]}}'
else
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Done."},{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
fi
`), 0o755))
	log := filepath.Join(scratch, "log.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	var got []string
	for _, m := range readJSONL(t, log) {
		if n := resultName(m); n != "" {
			got = append(got, n)
		}
	}
	sort.Strings(got)
	assert.Equal(t, want.results, got, "the mock runs the same hooks as the recording")
}
