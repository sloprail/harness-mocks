package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --max-turns N ends a run that would take more than N model turns with an error
// result, exit 1 and no Stop (cli-reference#--max-turns; recorded in
// runs/max-turns, two tool calls then the third turn refused): the mock's run of
// the same three calls gives the recorded result's fields and subtype, and the
// recorded hook events.
// sr:proves noninteractive-run/claude
func TestT001_11_MaxTurnsEndsTheRunWithAnErrorResult(t *testing.T) {
	recDir, err := filepath.Glob(filepath.Join("..", "..", "snapshots", "runs", "max-turns", "samples", "*"))
	require.NoError(t, err)
	require.Len(t, recDir, 1)
	recStream, err := os.ReadFile(filepath.Join(recDir[0], "stream.jsonl"))
	require.NoError(t, err)
	want := lastResult(t, string(recStream))
	exit, _ := os.ReadFile(filepath.Join(recDir[0], "exit.txt"))
	assert.Equal(t, "1", strings.TrimSpace(string(exit)))
	recPayloads, err := os.ReadFile(filepath.Join(recDir[0], "payloads.jsonl"))
	require.NoError(t, err)

	dir := t.TempDir()
	log := filepath.Join(dir, "hooks.log")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >> "+log+"\necho >> "+log+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	var h []string
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "SessionEnd"} {
		h = append(h, `"`+ev+`":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]`)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{"hooks":{`+strings.Join(h, ",")+`}}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_result"' "$A10N_MOCK_SESSION_FILE")
case $n in
0) echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t0","name":"Bash","input":{"command":"echo a"}}]}}';;
1) echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"echo b"}}]}}';;
2) echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"echo c"}}]}}';;
esac
`), 0o755))
	out, code := runInDirWithEnv(t, dir, nil, "--script", script, "--session-id", "max-1", "--project-dir", dir, "--output-format", "stream-json", "--max-turns", "2", "-p", "go")
	assert.Equal(t, 1, code, out)
	got := lastResult(t, out)
	for _, k := range []string{"subtype", "is_error", "num_turns", "stop_reason", "terminal_reason", "errors"} {
		assert.Equal(t, want[k], got[k], k)
	}
	assert.NotContains(t, got, "result")
	assert.Equal(t, 2, strings.Count(out, `"type":"tool_result"`), "the third call never ran")
	raw, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, hookEvents(t, string(recPayloads)), hookEvents(t, string(raw)), "the recorded hook events: no Stop")
}
