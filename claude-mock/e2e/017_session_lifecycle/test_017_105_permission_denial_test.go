package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamFrames(t *testing.T, out string) (frames []map[string]any) {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if strings.HasPrefix(l, "{") && json.Unmarshal([]byte(l), &f) == nil {
			frames = append(frames, f)
		}
	}
	return frames
}

// TestT017_105_DenyRuleRefusesTheCall: a deny rule of the settings refuses a Bash call of
// exactly that command, even under --dangerously-skip-permissions: the PreToolUse hooks have
// seen the call, the command does not run and no PostToolUse fires; the stream carries a
// permission_denied frame ahead of the tool result "Permission to use Bash with command <c>
// has been denied.", and the result lists the call in permission_denials. Another command
// still runs (recording permission-denied).
// sr:proves noninteractive-run/claude
func TestT017_105_DenyRuleRefusesTheCall(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	marker := filepath.Join(dir, "ran")
	h := payloadLogger(t, dir, "log.sh", log, "")
	write(t, filepath.Join(dir, ".claude", "settings.json"), `{"permissions":{"deny":["Bash(echo DENIED)"]},"hooks":{
"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+h+`"}]}],
"PostToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+h+`"}]}]}}`, 0o644)
	sc := script(t, dir, "s",
		toolUse("d1", "Bash", `{"command":"echo DENIED"}`),
		toolUse("d2", "Bash", `{"command":"touch `+marker+`"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pd-1", "--project-dir", dir, "--config-dir", cfg,
		"--output-format", "stream-json", "--dangerously-skip-permissions", "-p", "hello")
	require.Equal(t, 0, code, out)
	frames := streamFrames(t, out)

	var kinds []string
	var denied, result map[string]any
	for _, f := range frames {
		switch {
		case f["subtype"] == "permission_denied":
			denied = f
			kinds = append(kinds, "permission_denied")
		case f["type"] == "user":
			kinds = append(kinds, "tool_result")
		case f["type"] == "result":
			result = f
		}
	}
	assert.Equal(t, []string{"permission_denied", "tool_result", "tool_result"}, kinds, "the frame precedes the refused call's result")
	want := "Permission to use Bash with command echo DENIED has been denied."
	assert.Equal(t, want, denied["message"])
	assert.Equal(t, "rule", denied["decision_reason_type"])
	assert.Equal(t, "Bash", denied["tool_name"])
	list := result["permission_denials"].([]any)
	require.Len(t, list, 1)
	assert.Equal(t, map[string]any{"command": "echo DENIED"}, list[0].(map[string]any)["tool_input"])
	assert.Equal(t, "Bash", list[0].(map[string]any)["tool_name"])
	assert.Contains(t, out, `"content":"`+want+`","is_error":true`)

	var events []string
	for _, p := range payloads(t, log) {
		events = append(events, p["hook_event_name"].(string)+":"+p["tool_input"].(map[string]any)["command"].(string))
	}
	assert.Equal(t, []string{"PreToolUse:echo DENIED", "PreToolUse:touch " + marker, "PostToolUse:touch " + marker}, events,
		"the hooks saw the denied call, no PostToolUse followed it")

	// as recorded: the same frame fields, and the denial in the result
	rec := recordedFile(t, "../../snapshots/runs/permission-denied/samples/*/stream.jsonl")
	data, err := os.ReadFile(rec)
	require.NoError(t, err)
	var recDenied map[string]any
	for _, f := range streamFrames(t, string(data)) {
		if f["subtype"] == "permission_denied" {
			recDenied = f
		}
	}
	for k := range recDenied {
		assert.Contains(t, denied, k, "the recorded frame's field")
	}
	assert.Equal(t, recDenied["decision_reason_type"], denied["decision_reason_type"])
}

// TestT017_106_UnmodelledDenyRuleIsRefused: a deny rule other than an exact Bash command
// (a wildcard, another tool) is refused with the rule named, rather than ignored.
// sr:proves noninteractive-run/claude
func TestT017_106_UnmodelledDenyRuleIsRefused(t *testing.T) {
	for _, rule := range []string{"Bash(echo:*)", "Read(./secrets)", "Bash"} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, ".claude", "settings.json"), `{"permissions":{"deny":["`+rule+`"]}}`, 0o644)
		out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "pr-1", "--project-dir", dir,
			"--config-dir", filepath.Join(dir, "config"), "-p", "hello")
		assert.NotEqual(t, 0, code, rule)
		assert.Contains(t, out, rule, "the refusal names the rule")
		assert.Contains(t, out, "not implemented")
	}
}

// TestT017_107_AllowManagedHooksOnlyIsRefused: allowManagedHooksOnly, which the docs say blocks
// the hooks of plugins the managed settings do not force-enable, is refused wherever the
// settings set it, by name, rather than ignored (the managed settings are root-owned: no
// run of it could be recorded).
// sr:proves plugin-hooks/claude
func TestT017_107_AllowManagedHooksOnlyIsRefused(t *testing.T) {
	for _, file := range []string{"settings.json", "settings.local.json"} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, ".claude", file), `{"allowManagedHooksOnly":true}`, 0o644)
		out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "mh-1", "--project-dir", dir,
			"--config-dir", filepath.Join(dir, "config"), "-p", "hello")
		assert.NotEqual(t, 0, code, file)
		assert.Contains(t, out, "allowManagedHooksOnly")
		assert.Contains(t, out, "not implemented")
	}
}

// TestT017_108_APIRetryEventIsRefused: a scenario that streams the system/api_retry event is
// refused by name, rather than passed on as if the run had retried a model request.
// sr:proves noninteractive-run/claude
func TestT017_108_APIRetryEventIsRefused(t *testing.T) {
	dir := t.TempDir()
	sc := write(t, filepath.Join(dir, "s.sh"), "#!/bin/sh\necho '{\"type\":\"system\",\"subtype\":\"api_retry\",\"attempt\":1}'\n", 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "ar-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	assert.NotEqual(t, 0, code, out)
	assert.Contains(t, out, "system/api_retry event is not implemented by the mock")
	assert.NotContains(t, out, `"attempt"`)
}

// TestT017_113_MonitorAndWorkflowToolsAreRefused: a scenario that calls the Monitor or the
// Workflow tool is refused by name before the tool runs, rather than answered as an unknown tool.
// sr:proves print-waits-for-background-agents/claude
func TestT017_113_MonitorAndWorkflowToolsAreRefused(t *testing.T) {
	for _, tool := range []string{"Monitor", "Workflow"} {
		dir := t.TempDir()
		sc := script(t, dir, "s", toolUse("m1", tool, `{"command":"true"}`))
		out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "mw-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
		assert.NotEqual(t, 0, code, tool)
		assert.Contains(t, out, "the "+tool+" tool is not implemented by the mock", tool)
	}
}
