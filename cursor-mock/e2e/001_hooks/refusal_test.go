package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// runOneCall runs the mock on a script that makes one tool call and then ends,
// in a workspace holding files, and returns what it printed and whether it failed.
func runOneCall(t *testing.T, call string, files map[string]string, args ...string) (string, error) {
	t.Helper()
	ws, scratch := t.TempDir(), t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(ws, name), []byte(content), 0o644))
	}
	script := filepath.Join(scratch, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"`+call+`}]}}'; else printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}'; fi
`), 0o755))
	cmd := exec.Command(binary, append([]string{"-p", "--trust", "--output-format", "stream-json", "--script", script}, append(args, "go")...)...)
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// A Task call naming a sub-agent model no recording shows (the recorded ones
// are default, inherit and the invalid no-such-model-xyz) is refused as not
// modeled and fails the run, whatever the name.
// sr:proves foreground-subagent-result/cursor
func TestAnUnrecordedSubAgentModelIsRefused(t *testing.T) {
	for _, model := range []string{"gpt-5", "claude-sonnet", "fast"} {
		out, err := runOneCall(t, `Task","input":{"description":"d","prompt":"p","subagent_type":"generalPurpose","model":"`+model+`","script":"/nonexistent"}`, nil, "--force")
		require.Error(t, err, model+": a refusal of something not modeled fails the run")
		require.Contains(t, out, "sub-agent model", model)
		require.Contains(t, out, "is not modeled", model)
	}
}

// A Grep call with a key beyond the pattern fails the whole run: no recording
// shows what the harness does with one, so the mock refuses it.
// sr:proves hook-matcher-filter/cursor
func TestAGrepWithKeysBeyondThePatternFailsTheRun(t *testing.T) {
	out, err := runOneCall(t, `Grep","input":{"pattern":"hi","glob":"*.txt","-i":true}`, map[string]string{"a.txt": "hi\n"}, "--force")
	require.Error(t, err, out)
	require.Contains(t, out, `Grep: unknown parameter "glob"`, "the schema names the key the mock does not implement")
}

// What a file holds or a hook prints is not the mock's own refusal: a Read of a
// file that says "cursor-mock: " in a JSON string does not fail the run.
// sr:proves file-tools/cursor
func TestAFileThatQuotesTheMocksRefusalWordingIsNotARefusal(t *testing.T) {
	out, err := runOneCall(t, `Read","input":{"file_path":"a.txt"}`, map[string]string{"a.txt": "x\":\"cursor-mock: nothing\"\n"}, "--force")
	require.NoError(t, err, out)
	require.Contains(t, out, `"success"`, "the read succeeded")
}

// --add-dir is recorded only with --force: with --yolo, which no recording of
// it covers, the mock refuses it rather than guess.
// sr:proves noninteractive-run/cursor
func TestAddDirIsRefusedWithYolo(t *testing.T) {
	out, err := runOneCall(t, `Read","input":{"file_path":"a.txt"}`, map[string]string{"a.txt": "hi\n"}, "--yolo", "--add-dir", t.TempDir())
	require.Error(t, err, out)
	require.Contains(t, out, "--add-dir is modeled only with --force")
}

// A script's call of mcp__, mcp__x or mcp__x__ is not a call of any server's
// tool (a name is mcp__<server>__<tool>, both parts non-empty): it is refused
// as an unknown tool and fails the run, where no recording shows an answer.
// sr:proves hook-matcher-filter/cursor
func TestAnMCPNameWithoutAServerAndAToolIsRefusedAsUnknown(t *testing.T) {
	for _, name := range []string{"mcp__", "mcp__x", "mcp__x__"} {
		out, err := runOneCall(t, name+`","input":{}`, nil, "--force")
		require.Error(t, err, name+": "+out)
		require.Contains(t, out, name+": unknown tool", name)
	}
}
