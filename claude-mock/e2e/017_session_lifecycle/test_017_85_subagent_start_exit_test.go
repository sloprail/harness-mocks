package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hookerrors run (a hook command that exits 2 on SessionStart and
// SubagentStart, 1 on Stop): SubagentStart cannot block, so the foreground
// sub-agent still runs, SubagentStop fires, and the Agent call hands back the
// sub-agent's report as completed. The mock runs the recording's own setup (its
// settings and hook command) with the sub-agent calls the recording shows, and
// its hook events and its Agent result are compared with the recorded ones.
// sr:proves foreground-subagent-result/claude
func TestT017_85_SubagentStartExit2StillHandsBackTheReport(t *testing.T) {
	setup := filepath.Join("..", "..", "snapshots", "runs", "hookerrors", "setup")
	recorded := recordedHooks(t, "hookerrors")
	var call, want map[string]any
	var wantEvents []string
	for _, p := range recorded {
		wantEvents = append(wantEvents, p["hook_event_name"].(string)+"/"+str(p["tool_name"])+"/"+str(p["agent_type"]))
		if p["tool_name"] == "Agent" && p["hook_event_name"] == "PreToolUse" {
			call = p["tool_input"].(map[string]any)
		}
		if p["tool_name"] == "Agent" && p["hook_event_name"] == "PostToolUse" {
			want = p["tool_response"].(map[string]any)
		}
	}
	require.NotNil(t, call)
	require.NotNil(t, want)
	require.Equal(t, "completed", want["status"], "recorded")
	report := want["content"].([]any)[0].(map[string]any)["text"].(string)

	dir := t.TempDir()
	log := filepath.Join(dir, "payloads.log")
	for _, f := range []string{"hook.sh", "settings.json"} {
		body, err := os.ReadFile(filepath.Join(setup, f))
		require.NoError(t, err)
		dst := filepath.Join(dir, f)
		if f == "settings.json" {
			dst = filepath.Join(dir, ".claude", "settings.json")
		}
		write(t, dst, string(body), 0o755)
	}
	input := map[string]any{}
	for k, v := range call {
		input[k] = v
	}
	input["script"] = replyScript(t, dir, "sub", report)
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", string(raw)))
	out, code := runInDir(t, dir, []string{"HOOK_LOG=" + log}, "--script", orch, "--session-id", "hookerr-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	require.Equal(t, 0, code, out)

	var gotEvents []string
	var got map[string]any
	for _, p := range payloads(t, log) {
		gotEvents = append(gotEvents, p["hook_event_name"].(string)+"/"+str(p["tool_name"])+"/"+str(p["agent_type"]))
		if p["tool_name"] == "Agent" && p["hook_event_name"] == "PostToolUse" {
			got = p["tool_response"].(map[string]any)
		}
	}
	assert.Equal(t, wantEvents, gotEvents, "the hooks fire as recorded, the failing ones included")
	require.NotNil(t, got)
	for _, k := range []string{"status", "content", "harnessNoteCount", "harnessTailCount", "totalToolUseCount"} {
		assert.Equal(t, want[k], got[k], k)
	}
	assert.Equal(t, keysOf(want), keysOf(got))
}

func str(v any) string { s, _ := v.(string); return s }
