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

// A run whose model API fails (an unrecognised model, recorded in
// snapshots/runs/run-failure) streams the failure as an assistant message and
// an error result (is_error true), fires SessionStart, UserPromptSubmit and
// SessionEnd but no Stop, and exits 1. A script whose result frame says
// is_error true is that run.
// sr:proves noninteractive-run/claude
func TestT001_10_AFailedRunExitsOneWithItsErrorResult(t *testing.T) {
	recDir := filepath.Join("..", "..", "snapshots", "runs", "run-failure", "samples")
	exits, err := filepath.Glob(filepath.Join(recDir, "*", "exit.txt"))
	require.NoError(t, err)
	require.Len(t, exits, 1)
	exit, _ := os.ReadFile(exits[0])
	assert.Equal(t, "1", strings.TrimSpace(string(exit)), "recorded: the failed run exits 1")
	recStream, err := os.ReadFile(filepath.Join(filepath.Dir(exits[0]), "stream.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, true, lastResult(t, string(recStream))["is_error"], "recorded: an error result")
	recPayloads, err := os.ReadFile(filepath.Join(filepath.Dir(exits[0]), "payloads.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, []string{"SessionStart", "UserPromptSubmit", "SessionEnd"}, hookEvents(t, string(recPayloads)), "recorded: no Stop")

	dir := t.TempDir()
	log := filepath.Join(dir, "hooks.log")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >> "+log+"\necho >> "+log+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	var h strings.Builder
	for i, ev := range []string{"SessionStart", "UserPromptSubmit", "Stop", "SessionEnd"} {
		if i > 0 {
			h.WriteString(",")
		}
		h.WriteString(`"` + ev + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]`)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{"hooks":{`+h.String()+`}}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"There is an issue with the selected model."}]},"error":"model_not_found","is_api_error_message":true}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":true,"api_error_status":404,"terminal_reason":"api_error","result":"There is an issue with the selected model."}'
`), 0o755))
	out, code := runInDirWithEnv(t, dir, nil, "--script", script, "--session-id", "failed-1", "--project-dir", dir, "--output-format", "stream-json", "-p", "go")
	assert.Equal(t, 1, code, out)
	res := lastResult(t, out)
	assert.Equal(t, true, res["is_error"])
	assert.Equal(t, "There is an issue with the selected model.", res["result"])
	assert.Equal(t, float64(404), res["api_error_status"])
	raw, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, []string{"SessionStart", "UserPromptSubmit", "SessionEnd"}, hookEvents(t, string(raw)), "no Stop")
}

// lastResult is the last result frame of a stream.
func lastResult(t *testing.T, stream string) map[string]any {
	t.Helper()
	var res map[string]any
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["type"] == "result" {
			res = f
		}
	}
	require.NotNil(t, res, stream)
	return res
}

// hookEvents are the hook_event_name of each JSON payload line.
func hookEvents(t *testing.T, payloads string) (events []string) {
	t.Helper()
	for _, l := range strings.Split(payloads, "\n") {
		var p map[string]any
		if json.Unmarshal([]byte(l), &p) == nil && p["hook_event_name"] != nil {
			events = append(events, p["hook_event_name"].(string))
		}
	}
	return
}
