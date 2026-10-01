package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_55_BashResults: a foreground command's result is its output (the
// trailing newline trimmed), with the structured form {stdout, stderr,
// interrupted, isImage, noOutputExpected} for the hooks; a command that exits
// non-zero is an error that states "Exit code N" and then the output, and runs
// PostToolUseFailure with that text (recorded: snapshots/runs/bashfail), except
// that an exit status 1 of a command that only reports no match or a difference
// (grep, diff, test) is a valid result (docs, Bash tool behavior); any other
// command exiting 1, and any other status, is an error. Empty output is the
// placeholder.
// sr:proves bash-tool-result/claude
func TestT017_55_BashResults(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PostToolUse": h, "PostToolUseFailure": h})
	write(t, filepath.Join(dir, "in.txt"), "alpha\n", 0o644)
	cmds := []string{
		`echo hello; echo`,
		`echo OUT-LINE; echo ERR-LINE >&2; exit 3`,
		`true`,
		`grep nomatch in.txt`,
		`grep alpha in.txt | grep nomatch`,
		`diff in.txt /dev/null`,
		`test -f nothing.txt`,
		`sh -c 'exit 1'`,
		`grep nomatch missing-file.txt`,
	}
	var calls []string
	for i, c := range cmds {
		calls = append(calls, toolUse(fmt.Sprintf("c%d", i), "Bash", `{"command":`+fmt.Sprintf("%q", c)+`}`))
	}
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", calls...), "--session-id", "bt-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "bt-1"))
	for i, tc := range []struct {
		want     string // the whole result, or
		contains string // a part of it
		isErr    bool
	}{
		{want: "hello"},
		{want: "Exit code 3\nOUT-LINE\nERR-LINE", isErr: true},
		{want: "(Bash completed with no output)"},
		{want: "(Bash completed with no output)"}, // grep: no match, status 1
		{want: "(Bash completed with no output)"}, // the last command of the pipeline is a grep
		{contains: "alpha"},                       // diff prints the difference and exits 1: a valid result
		{want: "(Bash completed with no output)"}, // test
		{want: "Exit code 1", isErr: true},        // sh is no search: exit 1 is a failure
		{want: "Exit code 2\ngrep: missing-file.txt: No such file or directory", isErr: true},
	} {
		block, r := toolResultOf(t, recs, fmt.Sprintf("c%dturn-s-%s", i, string(rune('a'+i))))
		got := fmt.Sprint(block["content"])
		if tc.contains != "" {
			assert.Contains(t, got, tc.contains, cmds[i])
		} else {
			assert.Equal(t, tc.want, got, cmds[i])
		}
		assert.Equal(t, tc.isErr, block["is_error"] == true, cmds[i])
		if tc.isErr {
			want, _ := json.Marshal("Error: " + tc.want)
			assert.Contains(t, r.Raw, `"toolUseResult":`+string(want), "the transcript records the error as text")
		}
	}
	var failures []string
	var structured []map[string]any
	for _, p := range payloads(t, log) {
		if p["hook_event_name"] == "PostToolUseFailure" {
			failures = append(failures, fmt.Sprint(p["error"]))
		} else {
			structured = append(structured, p["tool_response"].(map[string]any))
		}
	}
	assert.Equal(t, []string{"Exit code 3\nOUT-LINE\nERR-LINE", "Exit code 1", "Exit code 2\ngrep: missing-file.txt: No such file or directory"}, failures)
	require.GreaterOrEqual(t, len(structured), 6)
	assert.Equal(t, map[string]any{"stdout": "hello", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false}, structured[0])
	assert.Equal(t, "", structured[1]["stdout"], "true: nothing printed")
	assert.Len(t, structured, 6, "hello, true, grep, grep, diff, test ran successfully: PostToolUse for each")
}

// TestT017_56_AgentDispatchWithoutRequiredInputIsRefusedBeforeAnyHook: a
// dispatch lacking its description or prompt is answered with the input
// validation error naming each missing parameter, before any hook: the
// PreToolUse hook never fires, no sub-agent starts, nothing runs (recorded:
// snapshots/runs/agent-invalid-input); a complete dispatch does run.
// sr:proves agent-input-validation/claude
func TestT017_56_AgentDispatchWithoutRequiredInputIsRefusedBeforeAnyHook(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PreToolUse": h, "SubagentStart": h, "PostToolUse": h, "PostToolUseFailure": h})
	ran := filepath.Join(dir, "sub-ran")
	sub := write(t, filepath.Join(dir, "sub.sh"), "#!/bin/sh\necho ran >> "+ran+"\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"HELPED\"}'\n", 0o755)
	s := `"script":"` + sub + `"`
	sc := script(t, dir, "s",
		toolUse("a0", "Agent", `{"description":"helper","subagent_type":"general-purpose",`+s+`}`),
		toolUse("a1", "Agent", `{"prompt":"go",`+s+`}`),
		toolUse("a2", "Agent", `{`+s+`}`),
		toolUse("a3", "Agent", `{"description":"helper","prompt":"go",`+s+`}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "ai-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "ai-1"))
	for i, want := range []string{
		"<tool_use_error>InputValidationError: Agent failed due to the following issue:\nThe required parameter `prompt` is missing</tool_use_error>",
		"<tool_use_error>InputValidationError: Agent failed due to the following issue:\nThe required parameter `description` is missing</tool_use_error>",
		"<tool_use_error>InputValidationError: Agent failed due to the following issues:\nThe required parameter `description` is missing\nThe required parameter `prompt` is missing</tool_use_error>",
	} {
		block, r := toolResultOf(t, recs, fmt.Sprintf("a%dturn-s-%s", i, string(rune('a'+i))))
		assert.Equal(t, want, block["content"])
		assert.Equal(t, true, block["is_error"])
		assert.Contains(t, r.Raw, `"toolUseResult":"InputValidationError: [`, "the issue list is recorded")
		assert.Contains(t, r.Raw, `invalid_type`)
	}
	var seen []string
	for _, p := range payloads(t, log) {
		if id, _ := p["tool_use_id"].(string); id != "" {
			seen = append(seen, p["hook_event_name"].(string)+" "+id[:2])
		}
	}
	assert.Equal(t, []string{"PreToolUse a3", "PostToolUse a3"}, seen, "only the complete dispatch reached the hooks")
	data, err := os.ReadFile(ran)
	require.NoError(t, err)
	assert.Equal(t, "ran\n", string(data), "the sub-agent ran once: for the complete dispatch only")
}
