package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_61_BackgroundBashHookPayloads: a Bash command run in the background
// reaches PreToolUse with its tool_input as the model wrote it, including
// run_in_background: true, and PostToolUse with the recorded tool_response of
// a command that answered at once: empty stdout and stderr, interrupted,
// isImage and noOutputExpected false, and a backgroundTaskId (recorded:
// snapshots/runs/bgbash and snapshots/runs/midturn, payloads.jsonl).
// sr:proves background-bash/claude
func TestT017_61_BackgroundBashHookPayloads(t *testing.T) {
	// bgbash recorded the input with a description ('long sleep'), midturn
	// without one (their payloads.jsonl; the normalized events.jsonl leaves
	// descriptions out): each recorded input is replayed as the model wrote it,
	// and the payload carries it back whichever form it is.
	for _, run := range []string{"bgbash", "midturn"} {
		in := recordedBackgroundBashInput(t, run)
		require.Equal(t, true, in["run_in_background"], "the recording is of a background call")
		t.Run(run, func(t *testing.T) { backgroundBashHookPayloads(t, in) })
	}
}

// recordedBackgroundBashInput is the tool_input of the first background Bash
// call the real claude made in a recorded run.
func recordedBackgroundBashInput(t *testing.T, run string) map[string]any {
	t.Helper()
	for _, p := range recordedHooks(t, run) {
		if in, _ := p["tool_input"].(map[string]any); p["hook_event_name"] == "PreToolUse" && p["tool_name"] == "Bash" && in["run_in_background"] == true {
			return in
		}
	}
	t.Fatalf("run %s has no background Bash call", run)
	return nil
}

func backgroundBashHookPayloads(t *testing.T, want map[string]any) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	logger := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PreToolUse": logger, "PostToolUse": logger})
	root := script(t, dir, "root", toolUse("bgh", "Bash", mustJSON(t, want)))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "bgh-1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, out)

	var pre, post map[string]any
	for _, p := range payloads(t, log) {
		id, _ := p["tool_use_id"].(string)
		if p["tool_name"] != "Bash" || len(id) < 3 || id[:3] != "bgh" {
			continue
		}
		switch p["hook_event_name"] {
		case "PreToolUse":
			pre = p
		case "PostToolUse":
			post = p
		}
	}
	require.NotNil(t, pre, "no PreToolUse for the background Bash call")
	require.NotNil(t, post, "no PostToolUse for the background Bash call")

	assert.Equal(t, want, pre["tool_input"],
		"PreToolUse carries run_in_background: true")

	resp, _ := post["tool_response"].(map[string]any)
	require.NotNil(t, resp)
	id, _ := resp["backgroundTaskId"].(string)
	assert.NotEmpty(t, id)
	assert.Equal(t, map[string]any{"stdout": "", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false, "backgroundTaskId": id}, resp)
	assert.Equal(t, pre["tool_input"], post["tool_input"], "PostToolUse repeats the same tool_input")
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
