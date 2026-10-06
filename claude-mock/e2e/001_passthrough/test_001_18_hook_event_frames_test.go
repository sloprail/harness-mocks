package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
)

func hookFramesOnly(stream string) (out []string) {
	for _, f := range frameOrder(stream) {
		if strings.HasPrefix(f, "hook_") {
			out = append(out, f)
		}
	}
	return
}

// writeEventSettings wires every given event to one command that logs its input.
func writeEventSettings(t *testing.T, dir string, events ...string) {
	t.Helper()
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\n"), 0o755))
	var parts []string
	for _, e := range events {
		parts = append(parts, `"`+e+`":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]`)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{"hooks":{`+strings.Join(parts, ",")+`}}`), 0o644))
}

// With --include-hook-events a failed tool's PostToolUseFailure and a foreground sub-agent's
// SubagentStart and SubagentStop hooks stream their frames too, the SubagentStart response after
// the sub-agent's first frame and SubagentStop's before the agent's task_updated; SessionEnd's
// hook leaves none (runs/include-hook-events-more).
// sr:proves noninteractive-run/claude
func TestT001_18_IncludeHookEventsStreamsFailureAndSubagentHooks(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, filepath.Join("..", "..", "snapshots", "runs", "include-hook-events-more", "samples", "*", "stream.jsonl")))
	require.NoError(t, err)
	want := frameOrder(string(raw))
	require.Contains(t, want, "hook_started/SubagentStart:general-purpose")
	require.Contains(t, want, "hook_started/PostToolUseFailure:Bash")
	require.NotContains(t, strings.Join(want, " "), "SessionEnd")

	dir := t.TempDir()
	writeEventSettings(t, dir, "SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "Stop", "SubagentStart", "SubagentStop", "SessionEnd")
	sub := writeScript(t, `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"HELPED"}]}}'
echo '{"type":"result","subtype":"success","result":"HELPED"}'
`)
	main := writeScript(t, `#!/bin/sh
n=$(grep -c '"type":"tool_result"' "$A10N_MOCK_SESSION_FILE")
case "$n" in
0) echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"exit 3"}}]}}' ;;
1) echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"prompt":"Reply only HELPED","description":"helper","script":"`+sub+`"}}]}}' ;;
*) echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"DONE"}]}}'
   echo '{"type":"result","subtype":"success","result":"DONE"}' ;;
esac
`)
	out, code := e2etest.RunInDir(t, dir, nil, "--script", main, "--session-id", "he-1", "--project-dir", dir, "--include-hook-events",
		"--output-format", "stream-json", "-p", "go")
	require.Equal(t, 0, code, out)
	assert.Equal(t, hookFramesOnly(strings.Join(want, "\n")), hookFramesOnly(strings.Join(frameOrder(out), "\n")), "the hook frames, in order, as recorded")
}

// The flag is refused rather than streamed unrecorded when the run reaches a hook whose frames
// are not recorded: a compaction's, a worktree's, or a background sub-agent's SubagentStart.
// sr:proves noninteractive-run/claude
func TestT001_19_IncludeHookEventsRefusesAnUnrecordedHookThatFires(t *testing.T) {
	compact := writeScript(t, `#!/bin/sh
echo '{"type":"compact","summary":"s","trigger":"manual"}'
`)
	sub := writeScript(t, "#!/bin/sh\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"x\"}'\n")
	background := writeScript(t, `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"prompt":"p","description":"d","run_in_background":true,"script":"`+sub+`"}}]}}'
`)
	for name, tc := range map[string]struct{ script, event string }{
		"compaction":       {compact, "PreCompact"},
		"background agent": {background, "SubagentStart"},
	} {
		dir := t.TempDir()
		writeEventSettings(t, dir, tc.event)
		out, code := e2etest.RunInDir(t, dir, nil, "--script", tc.script, "--session-id", "hu-1", "--project-dir", dir, "--include-hook-events",
			"--output-format", "stream-json", "-p", "go")
		assert.NotEqual(t, 0, code, name+": "+out)
		assert.Contains(t, out, "--include-hook-events with a", name)
		assert.Contains(t, out, tc.event, name)
		assert.NotContains(t, out, `"subtype":"hook_started"`, name+": nothing unrecorded is streamed")
	}
	// a hook configured for an event that never fires is no matter (the recording's own settings)
	dir := t.TempDir()
	writeEventSettings(t, dir, "PreCompact", "PostCompact")
	out, code := e2etest.RunInDir(t, dir, nil, "--script", writeScript(t, "#!/bin/sh\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"x\"}'\n"),
		"--session-id", "hu-2", "--project-dir", dir, "--include-hook-events", "--output-format", "stream-json", "-p", "go")
	assert.Equal(t, 0, code, out)
}

// In raw --print mode too, the Stop hook's frames follow the flag.
// sr:proves noninteractive-run/claude
func TestT001_20_IncludeHookEventsInRawPrintModeStreamsStop(t *testing.T) {
	dir := t.TempDir()
	writeEventSettings(t, dir, "Stop")
	out, code := e2etest.RunInDir(t, dir, nil, "--script", writeScript(t, "#!/bin/sh\necho RAW\n"), "--session-id", "rp-1", "--project-dir", dir,
		"--include-hook-events", "--print", "go")
	require.Equal(t, 0, code, out)
	assert.Equal(t, []string{"hook_started/Stop", "hook_response/Stop"}, hookFramesOnly(out))
}
